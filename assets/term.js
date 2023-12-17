import { Terminal } from 'xterm';
import { WebLinksAddon } from '@xterm/addon-web-links';


const MSG = 0x1;
const RESIZE = 0x2;

const termOptions = {
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

const terminal = new Terminal(termOptions);
terminal.loadAddon(new WebLinksAddon());
terminal.open(document.getElementById('terminal'));
terminal.focus();

const progress = '/-\\|';
let progressIndex = 0;

function base64ToBytes(base64) {
  const binString = atob(base64);
  return Uint8Array.from(binString, (m) => m.codePointAt(0));
}

function decodeProtocol(buffer) {
  // validate buffer length
  if (buffer.length < 11) {
    throw new Error("Buffer too short "
      + buffer.length
      + " \"" + (new TextDecoder().decode(buffer))) + "\"";
  }

  let offset = 0;

  // A: command byte
  const command = buffer[offset];

  offset += 1;

  // B: counter (2 bytes, big endian)
  const counter = buffer[offset] << 8 | buffer[offset + 1];

  offset += 2;

  // C: payload length (32 bits, big endian)
  const payloadLength = (
    buffer[offset] << 24 |
    buffer[offset + 1] << 16 |
    buffer[offset + 2] << 8 |
    buffer[offset + 3]
  );

  offset += 4;

  // Validate payload length
  if (offset + payloadLength + 4 > buffer.length) {
    throw new Error("Invalid payload length");
  }

  // D: payload (array of bytes)
  const payload = buffer.slice(offset, offset + payloadLength);

  offset += payloadLength;

  // F: checksum (FNV-1a, 32 bits, big endian)
  const checksum = (
    buffer[offset] << 24 |
    buffer[offset + 1] << 16 |
    buffer[offset + 2] << 8 |
    buffer[offset + 3]
  );

  // TODO: verify checksum

  return {
    command,
    counter,
    payloadLength,
    payload,
  };
}

function connectWS() {
  const { host, pathname: path, protocol: proto } = window.location;
  const url = `${proto === 'https:' ? 'wss' : 'ws'}://${host}${path === '/' ? '' : path}/ws`
  const ws = new WebSocket(url);

  ws.binaryType = 'blob';

  ws.onopen = () => terminal.reset();

  ws.onmessage = ({ data }) => {
    const reader = new FileReader();
    reader.onload = () => {
      var array = new Uint8Array(reader.result);
      while (array.length > 0) {
        var { command, counter, payloadLength, payload } = decodeProtocol(array);
        switch (command) {
          case MSG:
            terminal.write(new TextDecoder().decode(payload));
            break;
          case RESIZE:
            const [cols, rows] = (new TextDecoder().decode(payload)).split(':');
            terminal.resize(+rows, +cols);
            //console.log(`Resized to ${cols}x${rows}`);
            break
          default:
            console.log("not implemented", array);
            break;
        }
        if (array.length <= payloadLength + 11) { // TODO: find a better way to verify if there is more data
          break;
        }
        array = array.slice(payloadLength + 11);
      }
    };
    reader.readAsArrayBuffer(data);
  };

  ws.onerror = () => ws.close();

  ws.onclose = () => {
    terminal.reset();
    terminal.write(`\x1b[2J\x1b[0;0HConnection closed.\r\nReconnecting… ${progress[progressIndex]}\r\n`);
    progressIndex = (progressIndex + 1) % progress.length;
    setTimeout(connectWS, 1000);
  };

  terminal.onKey(({ key }) => ws.send(key));
}

connectWS();
