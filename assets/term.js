const termOptions = {
    fontSize: 20,
    fontFamily: 'terminal,courier-new,courier,monospace',
    macOptionClickForcesSelection: true,
    macOptionIsMeta: true,
    theme: {
        background: '#000000',
        black: '#000000',
        blue: '#427ab3',
        brightBlack: '#686a66',
        brightBlue: '#84b0d8',
        brightCyan: '#37e6e8',
        brightGreen: '#99e343',
        brightMagenta: '#bc94b7',
        brightRed: '#f54235',
        brightWhite: '#f1f1f0',
        brightYellow: '#fdeb61',
        cursor: '#adadad',
        cyan: '#00a7aa',
        foreground: '#d4d4d4',
        green: '#5ea702',
        magenta: '#89658e',
        red: '#d81e00',
        white: '#dbded8',
        yellow: '#cfae00',
    },
};

const terminal = new Terminal(termOptions);
terminal.open(document.getElementById('terminal'));


function base64ToBytes(base64) {
  const binString = atob(base64);
  return Uint8Array.from(binString, (m) => m.codePointAt(0));
}

function bytesToBase64(bytes) {
  const binString = String.fromCodePoint(...bytes);
  return btoa(binString);
}

const progress = '/-\\|';
let progressIndex = 0;

function connectWS() {

    const ws = new WebSocket(`ws://${window.location.host}/ws`);

    ws.onopen = () => {
        terminal.clear();
        terminal.reset();
    };

    ws.onmessage = (event) => {
        terminal.write(base64ToBytes(event.data));
    };

    ws.onerror = () => {
        ws.close();
    };

    ws.onclose = () => {
        terminal.clear();
        terminal.reset();
        terminal.write('Connection closed.\r\nReconnecting… ');
        progressIndex = (progressIndex + 1) % progress.length;
        terminal.write("\b" + progress[progressIndex]);
        setTimeout(connectWS, 1000);
    };
}

connectWS();

const fitAddon = new FitAddon.FitAddon();
terminal.resize(80, 25);
fitAddon.fit();

// TODO: add zmodem addon support. Old school but cool. :D
