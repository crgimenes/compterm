import { Terminal } from '@xterm/xterm';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { registerIIP, makeReserver } from './iip.js';

const MSG = 0x1;
const RESIZE = 0x2;

const decoder = new TextDecoder();

const termOptions = {
  // compterm is strictly one-way: the viewer never sends anything back, so the
  // terminal accepts no input.
  disableStdin: true,
  // the decoration API used to render inline images (OSC 1337) is proposed.
  allowProposedApi: true,
  cursorBlink: false,
  fontSize: 20,
  fontFamily: 'terminal,courier-new,courier,monospace',
  macOptionClickForcesSelection: true,
  macOptionIsMeta: true,
  theme: {
    background: '#000000',
    black: '#000000',
    blue: '#0225c7',
    brightBlack: '#676767',
    brightBlue: '#6871ff',
    brightCyan: '#5ffdff',
    brightGreen: '#5ff967',
    brightMagenta: '#ff76ff',
    brightRed: '#ff6d67',
    brightWhite: '#fffeff',
    brightYellow: '#fefb67',
    cursor: '#adadad',
    cyan: '#00c5c7',
    foreground: '#d4d4d4',
    green: '#00c200',
    magenta: '#c930c7',
    red: '#c91b00',
    white: '#c7c7c7',
    yellow: '#c7c400',
  },
};

let terminal;
// reserveIIP appends the blank rows an inline image (OSC 1337) needs, so output
// after it flows below the image instead of behind it.
let reserveIIP = (b) => b;

// Frames that arrive before the terminal exists: the websocket connects in
// parallel with the font and theme loads, so the first RESIZE sizes the
// terminal at creation (no visible relayout) and the snapshot is written the
// moment it opens. Bounded: a snapshot is at most a few hundred KB.
const pending = { size: null, writes: [], bytes: 0 };
const pendingMax = 8 * 1024 * 1024;

function applyMSG(payload) {
  if (terminal) {
    terminal.write(reserveIIP(payload));
    return;
  }
  if (pending.bytes + payload.length > pendingMax) {
    return;
  }
  pending.writes.push(payload.slice());
  pending.bytes += payload.length;
}

function applyRESIZE(payload) {
  const [rows, cols] = decoder.decode(payload).split(':');
  if (terminal) {
    terminal.resize(+cols, +rows);
    return;
  }
  pending.size = { rows: +rows, cols: +cols };
}

const progress = '/-\\|';
let progressIndex = 0;

// fnv1a computes the FNV-1a 32-bit hash, matching the Go protocol package.
function fnv1a(bytes) {
  let hash = 0x811c9dc5;
  for (let i = 0; i < bytes.length; i++) {
    hash ^= bytes[i];
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

// decodeProtocol decodes one frame [cmd][len][payload][fnv32] and verifies its
// checksum, throwing on a short, truncated, or corrupt frame.
function decodeProtocol(buffer) {
  if (buffer.length < 9) {
    throw new Error('frame too short: ' + buffer.length);
  }

  const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
  const command = buffer[0];
  const payloadLength = view.getUint32(1, false);

  if (5 + payloadLength + 4 > buffer.length) {
    throw new Error('frame truncated');
  }

  const expected = view.getUint32(5 + payloadLength, false);
  const actual = fnv1a(buffer.subarray(0, 5 + payloadLength));
  if (expected !== actual) {
    throw new Error('checksum mismatch');
  }

  const payload = buffer.subarray(5, 5 + payloadLength);
  return { command, payloadLength, payload };
}

function connectWS() {
  const { host, pathname, protocol: proto } = window.location;
  // strip trailing slash so a subpath (e.g. /compterm/) yields /compterm/ws,
  // not /compterm//ws (which the server would redirect and break the upgrade).
  const base = pathname.replace(/\/+$/, '');
  const url = `${proto === 'https:' ? 'wss' : 'ws'}://${host}${base}/ws`;
  const ws = new WebSocket(url);

  // arraybuffer, never blob: a Blob needs an async FileReader per message,
  // and readers of different-sized messages can complete out of order — a
  // small live frame applied before the big attach snapshot scrolls a stale
  // screen and leaves displaced rows behind. An ArrayBuffer is delivered
  // synchronously, so frames apply strictly in arrival order.
  ws.binaryType = 'arraybuffer';

  ws.onopen = () => {
    if (terminal) {
      terminal.reset();
    }
  };

  ws.onmessage = ({ data }) => {
    let array = new Uint8Array(data);
    try {
      // A single websocket message may carry several concatenated frames.
      while (array.length >= 9) {
        const { command, payloadLength, payload } = decodeProtocol(array);
        switch (command) {
          case MSG:
            // pass raw bytes: xterm.js reassembles UTF-8 across writes, so a
            // multibyte glyph split across frames (common with image ANSI)
            // doesn't turn into replacement characters. reserveIIP inserts the
            // blank rows an inline image occupies before xterm parses them.
            applyMSG(payload);
            break;
          case RESIZE:
            applyRESIZE(payload);
            break;
          default:
            console.log('unknown command', command);
        }
        array = array.subarray(payloadLength + 9);
      }
    } catch (err) {
      console.log('frame decode error:', err.message);
    }
  };

  ws.onerror = () => ws.close();

  ws.onclose = () => {
    if (terminal) {
      terminal.reset();
      terminal.write(`\x1b[2J\x1b[0;0HConnection closed.\r\nReconnecting… ${progress[progressIndex]}\r\n`);
      progressIndex = (progressIndex + 1) % progress.length;
      document.title = 'compterm';
    }
    setTimeout(connectWS, 1000);
  };

  // No terminal.onData handler: the viewer never sends input back to the host.
}

// loadTheme fetches an optional palette from the server so the viewer can match
// the operator's terminal. Falls back to the built-in theme.
async function loadTheme() {
  try {
    const res = await fetch('theme.json');
    if (res.ok) return await res.json();
  } catch (e) {
    // ignore: use the built-in theme
  }
  return {};
}

// The script tag sits after the stylesheets and the #terminal element, so the
// DOM and the @font-face rule are ready at evaluation time: no need to wait
// for window.onload (which would also wait for the icon fetches). The
// websocket connects first — its snapshot buffers while font and theme load
// in parallel.
connectWS();

(async () => {
  const themeReady = loadTheme();

  // The font must be loaded before open(): xterm measures the cell grid at
  // that moment, and measuring with the fallback font makes the same session
  // render with different metrics whenever the network is slower than page
  // setup. On failure the fallback is at least consistent. The download
  // itself started with the page (preload in index.html), so this usually
  // resolves immediately.
  try {
    await document.fonts.load(`${termOptions.fontSize}px terminal`);
  } catch (e) {
    // no Font Loading API or font fetch failure: proceed with the fallback
  }

  // Born at the session's size when the snapshot won the race: the terminal
  // never opens at the default size just to be resized in front of the user.
  if (pending.size) {
    termOptions.rows = pending.size.rows;
    termOptions.cols = pending.size.cols;
  }

  terminal = new Terminal(termOptions);
  terminal.loadAddon(new WebLinksAddon());
  terminal.open(document.getElementById('terminal'));
  terminal.onTitleChange((title) => document.title = title);

  for (const w of pending.writes) {
    terminal.write(reserveIIP(w));
  }
  pending.writes.length = 0;
  pending.bytes = 0;

  // The theme stays off the critical path: colors apply at runtime when the
  // fetch lands (the built-in palette shows meanwhile), and the inline-image
  // pieces register here because they need imageScale from the same file.
  const cfg = await themeReady;
  const imageScale = typeof cfg.imageScale === 'number' ? cfg.imageScale : undefined;
  delete cfg.imageScale;
  terminal.options.theme = Object.assign({}, termOptions.theme, cfg);
  registerIIP(terminal, imageScale);
  reserveIIP = makeReserver(terminal, imageScale);
})();
