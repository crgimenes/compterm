// iip.js — iTerm2 inline image protocol (OSC 1337 ; File=...) for the viewer.
//
// xterm.js has no public API to draw an image into the cell buffer, and the
// published image addon reaches into xterm-5 internals that xterm 6 renamed,
// so it fails silently. Instead we render the image ourselves with the public
// decoration API: an overlay element anchored to a buffer line through a
// marker, which xterm positions, scrolls, and disposes along with that line.
//
// Drawing and reserving space are split:
//
//   * registerIIP installs an OSC 1337 handler that draws the image. It runs at
//     parse time, when the cursor is on the image's line, so the marker lands
//     in the right place. It must not reserve space itself: a write() from
//     inside the handler is queued behind every already-buffered frame, so the
//     blank lines would land after the following output instead of before it.
//
//   * makeReserver returns a stateful byte transform applied to the stream
//     before terminal.write. When it sees a complete OSC 1337 it appends the
//     rows the image will occupy as newlines, so they are parsed right after
//     the sequence and the following output flows below the image.
//
// The host's emulator (mterm) does not understand the sequence, so the image is
// never part of the snapshot: a viewer that joins after it was shown only sees
// it the next time it is emitted, exactly as iTerm2 keeps images in scrollback
// rather than in a redrawable screen.

const ESC = 0x1b;
const BEL = 0x07;
const ST_TAIL = 0x5c; // '\' — second byte of the ST terminator (ESC \)
const LF = 0x0a;
// ESC ] 1 3 3 7 ; — the OSC 1337 introducer.
const INTRO = [0x1b, 0x5d, 0x31, 0x33, 0x33, 0x37, 0x3b];

// parseArgs splits the iTerm2 "File=" argument list ("k=v;k=v;...") into a map.
function parseArgs(s) {
  const out = {};
  for (const pair of s.split(';')) {
    if (!pair) continue;
    const eq = pair.indexOf('=');
    if (eq < 0) {
      out[pair] = '';
      continue;
    }
    out[pair.slice(0, eq)] = pair.slice(eq + 1);
  }
  return out;
}

// base64ToBytes decodes a base64 string into a Uint8Array, tolerating the
// embedded newlines that `base64` emits when its output is not unwrapped.
function base64ToBytes(b64) {
  const bin = atob(b64.replace(/\s+/g, ''));
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) {
    bytes[i] = bin.charCodeAt(i);
  }
  return bytes;
}

// codesToString turns an array of byte values into a string in bounded chunks,
// avoiding the argument-count limit of String.fromCharCode.apply on big images.
function codesToString(codes) {
  let s = '';
  for (let i = 0; i < codes.length; i += 8192) {
    s += String.fromCharCode.apply(null, codes.slice(i, i + 8192));
  }
  return s;
}

// imageSize reads the pixel dimensions straight from the encoded bytes for the
// formats whose header carries them up front (PNG and GIF), returning null when
// the format is unknown so the caller falls back to explicit args or a default.
function imageSize(b) {
  // PNG: 8-byte signature, then IHDR with width/height as big-endian uint32.
  if (
    b.length >= 24 &&
    b[0] === 0x89 && b[1] === 0x50 && b[2] === 0x4e && b[3] === 0x47
  ) {
    const dv = new DataView(b.buffer, b.byteOffset, b.byteLength);
    return { w: dv.getUint32(16), h: dv.getUint32(20) };
  }
  // GIF: "GIF8", then logical screen width/height as little-endian uint16.
  if (b.length >= 10 && b[0] === 0x47 && b[1] === 0x49 && b[2] === 0x46) {
    const dv = new DataView(b.buffer, b.byteOffset, b.byteLength);
    return { w: dv.getUint16(6, true), h: dv.getUint16(8, true) };
  }
  return null;
}

// cellSize measures one character cell in CSS pixels from the rendered terminal,
// falling back to a rough estimate from the font size before the first render.
function cellSize(terminal) {
  const screen =
    terminal.element && terminal.element.querySelector('.xterm-screen');
  if (screen && terminal.cols && terminal.rows) {
    const w = screen.clientWidth / terminal.cols;
    const h = screen.clientHeight / terminal.rows;
    if (w > 0 && h > 0) {
      return { w, h };
    }
  }
  const fs = (terminal.options && terminal.options.fontSize) || 16;
  return { w: fs * 0.6, h: fs * 1.2 };
}

// dimToPixels resolves one iTerm2 size token (N cells, Npx, N%, or auto) to CSS
// pixels, or null when the token is absent/auto so the caller derives it from
// the natural size and aspect ratio. Explicit tokens are taken literally.
function dimToPixels(tok, cell, axisCells) {
  if (!tok || tok === 'auto') return null;
  if (tok.endsWith('px')) return parseFloat(tok);
  if (tok.endsWith('%')) return (parseFloat(tok) / 100) * axisCells * cell;
  return parseFloat(tok) * cell; // bare number = cells
}

// splitIIP separates an OSC 1337 payload ("File=k=v;...:base64") into its parsed
// argument map and the raw image bytes, or returns null when it is not an inline
// file payload.
function splitIIP(data) {
  if (!data.startsWith('File=')) return null;
  const colon = data.indexOf(':');
  if (colon < 0) return null;
  const b64 = data.slice(colon + 1).trim();
  if (!b64) return null;
  return { args: parseArgs(data.slice(5, colon)), bytes: base64ToBytes(b64) };
}

// layout computes the on-screen size of an image in both CSS pixels and whole
// terminal cells, honoring explicit width/height args and never exceeding the
// viewport width.
function layout(bytes, args, terminal, imageScale) {
  const cell = cellSize(terminal);
  // Auto-sizing scales the image's natural pixels. The default (1 / device
  // pixel ratio) matches iTerm2 on a Retina display — one device pixel per
  // image pixel — but a terminal whose font differs from this viewer's needs a
  // different ratio, so the operator can override it with theme.json imageScale.
  const scale = imageScale != null ? imageScale : 1 / (window.devicePixelRatio || 1);
  const natural = imageSize(bytes) || { w: 0, h: 0 };
  const ar = natural.w && natural.h ? natural.w / natural.h : 2;

  let wPx = dimToPixels(args.width, cell.w, terminal.cols);
  let hPx = dimToPixels(args.height, cell.h, terminal.rows);
  if (wPx == null && hPx == null) {
    wPx = natural.w * scale || 20 * cell.w;
    hPx = natural.h * scale || wPx / ar;
  } else if (wPx == null) {
    wPx = hPx * ar;
  } else if (hPx == null) {
    hPx = wPx / ar;
  }

  const maxW = terminal.cols * cell.w;
  if (wPx > maxW) {
    hPx *= maxW / wPx;
    wPx = maxW;
  }

  return {
    wPx,
    hPx,
    cols: Math.max(1, Math.min(terminal.cols, Math.ceil(wPx / cell.w))),
    rows: Math.max(1, Math.ceil(hPx / cell.h)),
  };
}

// registerIIP installs the OSC 1337 handler that draws inline images. It only
// renders; makeReserver is responsible for the layout. imageScale (from
// theme.json, may be undefined) tunes the auto-size.
export function registerIIP(terminal, imageScale) {
  terminal.parser.registerOscHandler(1337, (data) => {
    try {
      const parsed = splitIIP(data);
      if (!parsed) return data.startsWith('File='); // consume only File= forms

      const url = URL.createObjectURL(new Blob([parsed.bytes]));
      const { wPx, hPx, cols, rows } = layout(parsed.bytes, parsed.args, terminal, imageScale);

      const marker = terminal.registerMarker(0);
      if (!marker) {
        URL.revokeObjectURL(url);
        return true;
      }
      marker.onDispose(() => URL.revokeObjectURL(url));

      // Anchor at the cursor's column so images emitted in a row line up side by
      // side instead of stacking at the left edge.
      const active = terminal.buffer && terminal.buffer.active;
      const x = active ? active.cursorX : 0;
      const dec = terminal.registerDecoration({ marker, x, width: cols, height: rows });
      if (dec) {
        dec.onRender((el) => {
          if (el.dataset.iip) return;
          el.dataset.iip = '1';
          el.style.width = wPx + 'px';
          el.style.height = hPx + 'px';
          el.style.backgroundImage = `url(${url})`;
          el.style.backgroundSize = 'contain';
          el.style.backgroundRepeat = 'no-repeat';
          el.style.backgroundPosition = 'top left';
          el.style.pointerEvents = 'none';
          el.style.zIndex = '4';
        });
      }

      // The image sits on an otherwise-blank line, which xterm would not repaint
      // on an idle screen, so the decoration would never get its element. Force a
      // single viewport refresh once the write settles to draw it.
      setTimeout(() => terminal.refresh(0, terminal.rows - 1), 0);
      return true;
    } catch (e) {
      console.log('iip error:', e && e.message);
      return true; // consume malformed payloads instead of dumping base64
    }
  });
}

// makeReserver returns a stateful transform over the inbound byte stream that
// reproduces iTerm2's cursor behavior around an inline image: after a complete
// OSC 1337 ; File= sequence it moves the cursor to the image's bottom-right
// corner — down by its height (creating the rows it spans) and right by its
// width — so text after the image aligns with its base and a following line feed
// lands below it. A partial sequence at the end of a chunk is held until the
// rest arrives, so an image split across frames works.
export function makeReserver(terminal, imageScale) {
  let state = 0; // 0 normal, 1 matching introducer, 2 inside payload
  let pos = 0; // matched introducer bytes
  let held = []; // bytes withheld while a sequence is in progress
  let payload = []; // OSC 1337 data bytes (between introducer and terminator)
  let sawEsc = false; // inside payload, saw ESC awaiting ST '\'

  // sizeFor decodes the captured payload and returns the image's cell size, or
  // null when the payload is not a usable inline image.
  const sizeFor = (codes) => {
    try {
      const parsed = splitIIP(codesToString(codes));
      if (!parsed) return null;
      const { cols, rows } = layout(parsed.bytes, parsed.args, terminal, imageScale);
      return { cols, rows };
    } catch (e) {
      return null;
    }
  };

  return function reserve(input) {
    const out = [];
    for (let i = 0; i < input.length; i++) {
      const b = input[i];

      if (state === 0) {
        if (b === INTRO[0]) {
          held = [b];
          pos = 1;
          state = 1;
          continue;
        }
        out.push(b);
        continue;
      }

      if (state === 1) {
        if (b === INTRO[pos]) {
          held.push(b);
          pos++;
          if (pos === INTRO.length) {
            state = 2;
            payload = [];
            sawEsc = false;
          }
        } else {
          // not OSC 1337: replay the withheld bytes and reprocess this one
          for (let k = 0; k < held.length; k++) out.push(held[k]);
          held = [];
          state = 0;
          i--;
        }
        continue;
      }

      // state 2: inside the OSC 1337 payload, scanning for BEL or ST (ESC \).
      held.push(b);
      if (sawEsc) {
        if (b === ST_TAIL) {
          flush(out, held, sizeFor(payload));
          state = 0;
          held = [];
        } else {
          // a stray ESC inside the payload: not our terminator, give up cleanly
          for (let k = 0; k < held.length; k++) out.push(held[k]);
          state = 0;
          held = [];
        }
        sawEsc = false;
        continue;
      }
      if (b === BEL) {
        flush(out, held, sizeFor(payload));
        state = 0;
        held = [];
      } else if (b === ESC) {
        sawEsc = true;
      } else {
        payload.push(b);
      }
    }
    return new Uint8Array(out);
  };
}

// flush writes a completed OSC 1337 sequence, then moves the cursor to the
// image's bottom-right: down by height-1 rows (line feeds, which also create
// those rows) and right past its width (CSI Ps C). The extra column past the
// image's cell width keeps trailing text from butting against it. Text emitted
// next then sits at the image's base, beside it, as in iTerm2.
function flush(out, seq, size) {
  for (let k = 0; k < seq.length; k++) out.push(seq[k]);
  if (!size) return;
  let move = '';
  for (let r = 1; r < size.rows; r++) move += '\n';
  move += `\x1b[${size.cols + 1}C`;
  for (let k = 0; k < move.length; k++) out.push(move.charCodeAt(k));
}
