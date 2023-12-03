import { Terminal } from 'xterm';
import { WebLinksAddon } from '@xterm/addon-web-links';

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

const progress = '/-\\|';
let progressIndex = 0;

function connectWS() {
    const { host, pathname: path, protocol: proto } = window.location;
    const url = `${proto === 'https:' ? 'wss' : 'ws'}://${host}${path === '/' ? '' : path}/ws`
    const ws = new WebSocket(url);
    
    ws.binaryType = 'blob';

    ws.onopen = () => terminal.reset();

    ws.onmessage = ({ data }) => {
        const reader = new FileReader();
        reader.onload = () => {
            const array = new Uint8Array(reader.result);
            const params = array.slice(1);
            switch (array.slice(0, 1)[0]) {
                case 0x1:
                    terminal.write(params);
                    break;
                case 0x2:
                    const [cols, rows] = (new TextDecoder().decode(params)).split(':')
                    terminal.resize(+rows, +cols);
                    break
                default:
                    console.log("not implemented", array);
                    break;
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
