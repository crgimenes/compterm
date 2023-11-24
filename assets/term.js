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

    terminal.onKey(e => {
        ws.send(e.key);
    });


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
