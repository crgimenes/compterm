(() => {

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
    terminal.resize(80, 24);

    const progress = '/-\\|';
    let progressIndex = 0;

    function connectWS() {

        const ws = new WebSocket(`ws://${window.location.host}/ws`);
        ws.binaryType = 'blob';

        ws.onopen = () => terminal.reset();

        ws.onmessage = ({ data }) => {
            const reader = new FileReader();
            reader.onload = () => {
                const array = new Uint8Array(reader.result);
                switch (array.slice(0, 1)[0]) {
                    case 0x1:
                        terminal.write(array.slice(1));
                        break;

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

})();