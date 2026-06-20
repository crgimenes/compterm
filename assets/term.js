import { Terminal } from '@xterm/xterm';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { ImageAddon } from '@xterm/addon-image';

const MSG = 0x1;
const RESIZE = 0x2;

const decoder = new TextDecoder();

const termOptions = {
  // compterm is strictly one-way: the viewer never sends anything back, so the
  // terminal accepts no input.
  disableStdin: true,
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

// decodeProtocol decodes one frame [cmd][counter][len][payload][fnv32] and
// verifies its checksum, throwing on a short, truncated, or corrupt frame.
function decodeProtocol(buffer) {
  if (buffer.length < 11) {
    throw new Error('frame too short: ' + buffer.length);
  }

  const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
  const command = buffer[0];
  const counter = view.getUint16(1, false);
  const payloadLength = view.getUint32(3, false);

  if (7 + payloadLength + 4 > buffer.length) {
    throw new Error('frame truncated');
  }

  const expected = view.getUint32(7 + payloadLength, false);
  const actual = fnv1a(buffer.subarray(0, 7 + payloadLength));
  if (expected !== actual) {
    throw new Error('checksum mismatch');
  }

  const payload = buffer.subarray(7, 7 + payloadLength);
  return { command, counter, payloadLength, payload };
}

function connectWS() {
  const { host, pathname, protocol: proto } = window.location;
  // strip trailing slash so a subpath (e.g. /compterm/) yields /compterm/ws,
  // not /compterm//ws (which the server would redirect and break the upgrade).
  const base = pathname.replace(/\/+$/, '');
  const url = `${proto === 'https:' ? 'wss' : 'ws'}://${host}${base}/ws`;
  const ws = new WebSocket(url);

  ws.binaryType = 'blob';

  ws.onopen = () => terminal.reset();

  ws.onmessage = ({ data }) => {
    const reader = new FileReader();
    reader.onload = () => {
      let array = new Uint8Array(reader.result);
      try {
        // A single websocket message may carry several concatenated frames.
        while (array.length >= 11) {
          const { command, payloadLength, payload } = decodeProtocol(array);
          switch (command) {
            case MSG:
              // pass raw bytes: xterm.js reassembles UTF-8 across writes, so a
              // multibyte glyph split across frames (common with image ANSI)
              // doesn't turn into replacement characters.
              terminal.write(payload);
              break;
            case RESIZE: {
              const [cols, rows] = decoder.decode(payload).split(':');
              terminal.resize(+rows, +cols);
              break;
            }
            default:
              console.log('unknown command', command);
          }
          array = array.subarray(payloadLength + 11);
        }
      } catch (err) {
        console.log('frame decode error:', err.message);
      }
    };
    reader.readAsArrayBuffer(data);
  };

  ws.onerror = () => ws.close();

  ws.onclose = () => {
    terminal.reset();
    terminal.write(`\x1b[2J\x1b[0;0HConnection closed.\r\nReconnecting… ${progress[progressIndex]}\r\n`);
    progressIndex = (progressIndex + 1) % progress.length;
    document.title = 'compterm';
    setTimeout(connectWS, 1000);
  };

  // No terminal.onData handler: the viewer never sends input back to the host.
  terminal.onTitleChange((title) => document.title = title);
  terminal.onerror = (err) => console.log(err);
}

// loadTheme fetches an optional palette from the server so the viewer can match
// the operator's terminal. Falls back to the built-in theme.
async function loadTheme() {
  try {
    const res = await fetch('theme.json', { cache: 'no-store' });
    if (res.ok) return await res.json();
  } catch (e) {
    // ignore: use the built-in theme
  }
  return {};
}

window.onload = async () => {
  termOptions.theme = Object.assign({}, termOptions.theme, await loadTheme());

  terminal = new Terminal(termOptions);
  terminal.loadAddon(new WebLinksAddon());
  terminal.loadAddon(new ImageAddon());
  terminal.open(document.getElementById('terminal'));

  connectWS();
};
