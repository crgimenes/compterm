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

const ws = new WebSocket('ws://localhost:8080/ws');
const attachAddon = new AttachAddon.AttachAddon(ws);
terminal.loadAddon(attachAddon);
const fitAddon = new FitAddon.FitAddon();
terminal.resize(80, 25);
fitAddon.fit();

// TODO: add zmodem addon support. Old school but cool. :D