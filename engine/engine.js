/*
 * Marathi transliteration engine: Roman letters (the way people type Marathi
 * in chats) -> ranked Devanagari suggestions.
 *
 * How it works
 *  1. A lexicon of ~200k Marathi words with corpus frequencies (IndicCorp,
 *     via AI4Bharat IndicXlit, MIT).
 *  2. Every lexicon word is romanised by rules (with schwa deletion and the
 *     usual Marathi spelling habits: z for झ, w for व, dny for ज्ञ ...) and
 *     indexed under a loose phonetic key, so "mhanje", "mhnje" and "mhanaje"
 *     all reach म्हणजे.
 *  3. Candidates sharing the input's key are ranked by frequency plus how
 *     closely their romanisation matches what was typed.
 *  4. A curated table covers chat shorthand (mla, kr, nhi, pn ...), and a
 *     rule-based converter handles words that are not in the lexicon.
 *  5. The words a user picks are remembered and ranked first next time.
 *
 * This file has no dependencies and runs in browsers, extension service
 * workers and Node. The Windows app has a line-by-line Go port (engine.go);
 * tests/parity check that both produce identical output.
 */
(function (root, factory) {
  if (typeof module === 'object' && module.exports) module.exports = factory();
  else root.MarathiEngine = factory();
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  // plain lookup tables without Object.prototype keys ("constructor", ...)
  function dict(o) { var d = Object.create(null); if (o) for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) d[k] = o[k]; return d; }

  // ---------------------------------------------------------------------
  // Devanagari -> Roman (used to index the lexicon)
  // ---------------------------------------------------------------------
  var CONS = {
    'क': 'k', 'ख': 'kh', 'ग': 'g', 'घ': 'gh', 'ङ': 'n',
    'च': 'ch', 'छ': 'chh', 'ज': 'j', 'झ': 'z', 'ञ': 'n',
    'ट': 't', 'ठ': 'th', 'ड': 'd', 'ढ': 'dh', 'ण': 'n',
    'त': 't', 'थ': 'th', 'द': 'd', 'ध': 'dh', 'न': 'n', 'ऩ': 'n',
    'प': 'p', 'फ': 'ph', 'ब': 'b', 'भ': 'bh', 'म': 'm',
    'य': 'y', 'र': 'r', 'ऱ': 'r', 'ल': 'l', 'ळ': 'l', 'ऴ': 'l', 'व': 'v',
    'श': 'sh', 'ष': 'sh', 'स': 's', 'ह': 'h',
    'क़': 'k', 'ख़': 'kh', 'ग़': 'g', 'ज़': 'z', 'ड़': 'd', 'ढ़': 'dh', 'फ़': 'f', 'य़': 'y',
    '\u0958': 'k', '\u0959': 'kh', '\u095A': 'g', '\u095B': 'z', '\u095C': 'd', '\u095D': 'dh', '\u095E': 'f', '\u095F': 'y'
  };
  var NUKTA_BASE = { 'क': 'क़', 'ख': 'ख़', 'ग': 'ग़', 'ज': 'ज़', 'ड': 'ड़', 'ढ': 'ढ़', 'फ': 'फ़', 'य': 'य़' };
  var IND_VOWEL = {
    'अ': 'a', 'आ': 'a', 'इ': 'i', 'ई': 'i', 'उ': 'u', 'ऊ': 'u', 'ऋ': 'ru',
    'ए': 'e', 'ऎ': 'e', 'ऐ': 'ai', 'ओ': 'o', 'ऒ': 'o', 'औ': 'au',
    'ऍ': 'e', 'ॲ': 'a', 'ऑ': 'o'
  };
  var MATRA = {
    'ा': 'a', 'ि': 'i', 'ी': 'i', 'ु': 'u', 'ू': 'u', 'ृ': 'ru',
    'े': 'e', 'ॆ': 'e', 'ै': 'ai', 'ो': 'o', 'ॊ': 'o', 'ौ': 'au',
    'ॅ': 'a', 'ॉ': 'o'
  };
  var VIRAMA = '्', ANUSVARA = 'ं', CHANDRA = 'ँ', VISARGA = 'ः', NUKTA = '़';

  // Split a word into aksharas: {c: [consonant romans], v: vowel roman | 'A'
  // (inherent schwa) | '' (explicit virama), n: nasal, h: visarga}
  function parseWord(word) {
    var chars = Array.from(word.replace(/[‌‍]/g, ''));
    var aks = [];
    var cur = null;
    for (var i = 0; i < chars.length; i++) {
      var ch = chars[i];
      if (chars[i + 1] === NUKTA && NUKTA_BASE[ch]) { ch = NUKTA_BASE[ch]; i++; }
      if (CONS[ch] !== undefined) {
        if (cur && cur.v === '' && cur.pending) {
          cur.c.push(CONS[ch]); cur.cd.push(ch); cur.pending = false; cur.v = 'A';
        } else {
          cur = { c: [CONS[ch]], cd: [ch], v: 'A', n: false, h: false, pending: false };
          aks.push(cur);
        }
      } else if (IND_VOWEL[ch] !== undefined) {
        cur = { c: [], cd: [], v: IND_VOWEL[ch], n: false, h: false, pending: false, dv: ch };
        aks.push(cur);
      } else if (MATRA[ch] !== undefined) {
        if (cur && cur.c.length) { cur.v = MATRA[ch]; cur.dv = ch; cur.pending = false; }
      } else if (ch === VIRAMA) {
        if (cur && cur.c.length) { cur.v = ''; cur.pending = true; }
      } else if (ch === ANUSVARA || ch === CHANDRA) {
        if (cur) cur.n = true;
      } else if (ch === VISARGA) {
        if (cur) cur.h = true;
      }
    }
    return aks;
  }

  // Right-to-left schwa deletion (the Hindi/Marathi rule VC_CV).
  function schwaMask(aks, medial, keepFinalCluster) {
    var keep = aks.map(function (a) { return a.v === 'A'; });
    var n = aks.length;
    if (n > 1 && keep[n - 1] && aks[n - 1].c.length && !aks[n - 1].n && !aks[n - 1].h &&
        !(keepFinalCluster && aks[n - 1].c.length > 1)) keep[n - 1] = false;
    if (medial) {
      for (var i = n - 2; i >= 1; i--) {
        var a = aks[i];
        if (a.v !== 'A' || a.n || a.h || a.c.length !== 1) continue;
        var prev = aks[i - 1], next = aks[i + 1];
        var prevHasV = prev.v !== '' && (prev.v !== 'A' || keep[i - 1]);
        // next akshara must be a single consonant (or C+य as in च्या, ल्या) with a vowel
        var nextSimple = next.c.length === 1 || (next.c.length === 2 && next.c[1] === 'y');
        var nextHasV = nextSimple && next.v !== '' && (next.v !== 'A' || keep[i + 1]);
        if (prevHasV && nextHasV && next.c.length) keep[i] = false;
      }
    }
    return keep;
  }

  var LABIAL = { p: 1, ph: 1, b: 1, bh: 1, m: 1 };
  // opts: medial (bool), finalNasal (bool), dny ('dny'|'gy'|'gn'), ru ('ru'|'ri')
  function romanize(aks, opts) {
    var keep = schwaMask(aks, opts.medial, !opts.medial);
    var out = '';
    for (var i = 0; i < aks.length; i++) {
      var a = aks[i];
      var cons = a.c.join('');
      if (a.cd.length === 2 && a.cd[0] === 'ज' && a.cd[1] === 'ञ') cons = opts.dny;
      else if (a.cd.length === 2 && a.cd[0] === 'क' && a.cd[1] === 'ष') cons = 'ksh';
      out += cons;
      var v = a.v === 'A' ? (keep[i] ? 'a' : '') : a.v;
      if (a.v === 'ru') v = opts.ru; // कृ -> kru / kri
      out += v;
      if (a.n) {
        var last = i === aks.length - 1;
        if (!last) {
          var nx = aks[i + 1];
          out += (nx.c.length && LABIAL[nx.c[0]]) ? 'm' : 'n';
        } else if (opts.finalNasal) out += 'n';
      }
      if (a.h && opts.finalNasal) out += 'h';
    }
    return out;
  }

  function devToRomanVariants(word) {
    var aks = parseWord(word);
    if (!aks.length) return [];
    var hasDny = /ज्ञ/.test(word), hasRu = /[ृऋ]/.test(word);
    var seen = dict(), list = [];
    function add(o) { var r = romanize(aks, o); if (r && !seen[r]) { seen[r] = 1; list.push(r); } }
    var base = { medial: true, finalNasal: false, dny: 'dny', ru: 'ru' };
    add(base);
    add({ medial: false, finalNasal: false, dny: 'dny', ru: 'ru' });
    if (aks[aks.length - 1].n || /[ःँ]/.test(word)) add({ medial: true, finalNasal: true, dny: 'dny', ru: 'ru' });
    if (hasDny) {
      add({ medial: true, finalNasal: false, dny: 'gy', ru: 'ru' });
      add({ medial: true, finalNasal: false, dny: 'gn', ru: 'ru' });
      add({ medial: false, finalNasal: false, dny: 'gny', ru: 'ru' });
    }
    if (hasRu) add({ medial: true, finalNasal: false, dny: 'dny', ru: 'ri' });
    // single consonant words ("व", "न") are also typed without the vowel
    if (aks.length === 1 && aks[0].v === 'A' && aks[0].c.length === 1 && !aks[0].n && !seen[aks[0].c[0]]) { seen[aks[0].c[0]] = 1; list.push(aks[0].c[0]); }
    return list;
  }

  // ---------------------------------------------------------------------
  // Loose keys over Roman text
  // ---------------------------------------------------------------------
  function key1(s) {
    s = s.toLowerCase().replace(/[^a-z]/g, '');
    s = s.replace(/chh/g, 'ch').replace(/cch/g, 'ch').replace(/ksh/g, 'x').replace(/ph/g, 'f').replace(/jh/g, 'z')
      .replace(/w/g, 'v').replace(/q/g, 'k').replace(/c(?=[eiy])/g, 's').replace(/c(?!h)/g, 'k')
      .replace(/ou/g, 'au').replace(/ee/g, 'i').replace(/oo/g, 'u')
      .replace(/([aeiou])\1+/g, '$1')
      .replace(/([b-df-hj-np-tv-z])y(?![aeiou])/g, '$1i')    // army, company
      .replace(/ao(n?)$/, 'av');                              // gaon -> गाव
    return s;
  }
  function key2(s) {
    s = key1(s);
    s = s.replace(/sh/g, 's').replace(/x/g, 'ks').replace(/([kgcjtdpb])h/g, '$1').replace(/z/g, 'j')
      .replace(/iy(?=[aeiou])/g, 'i').replace(/iy$/, 'i').replace(/ay(?![aeiou])/g, 'e').replace(/([lnmrv])h/g, '$1')
      .replace(/m(?=[b-df-hj-np-tv-z])/g, 'n')
      .replace(/ai/g, 'e').replace(/au/g, 'o').replace(/ei/g, 'e')
      .replace(/([b-df-hj-np-tv-z])\1+/g, '$1');
    if (s.length > 1) s = s.charAt(0) + s.slice(1).replace(/a/g, '');
    s = s.replace(/([b-df-hj-np-tv-z])\1+/g, '$1');
    return s;
  }

  // Edit distance with cheap vowel-length / aspiration edits.
  function editDist(a, b) {
    var m = a.length, n = b.length;
    if (!m) return n; if (!n) return m;
    var prev = new Array(n + 1), cur = new Array(n + 1);
    for (var j = 0; j <= n; j++) prev[j] = j;
    for (var i = 1; i <= m; i++) {
      cur[0] = i;
      var ca = a.charCodeAt(i - 1);
      for (j = 1; j <= n; j++) {
        var cb = b.charCodeAt(j - 1);
        var sub = prev[j - 1] + (ca === cb ? 0 : 1);
        var da = (ca === 97 || ca === 104) ? 0.5 : 1; // 'a' / 'h' are cheap
        var db = (cb === 97 || cb === 104) ? 0.5 : 1;
        var del = prev[j] + da, ins = cur[j - 1] + db;
        cur[j] = sub < del ? (sub < ins ? sub : ins) : (del < ins ? del : ins);
      }
      var t = prev; prev = cur; cur = t;
    }
    return prev[n];
  }

  // ---------------------------------------------------------------------
  // Roman -> Devanagari rules (fallback for words not in the lexicon)
  // ---------------------------------------------------------------------
  var R_CONS = [
    ['ksh', 'क्ष'], ['dny', 'ज्ञ'], ['gny', 'ज्ञ'], ['chh', 'छ'], ['Sh', 'ष'], ['shh', 'ष'],
    ['kh', 'ख'], ['gh', 'घ'], ['ch', 'च'], ['jh', 'झ'], ['Th', 'ठ'], ['Dh', 'ढ'],
    ['th', 'थ'], ['dh', 'ध'], ['ph', 'फ'], ['bh', 'भ'], ['sh', 'श'],
    ['k', 'क'], ['g', 'ग'], ['j', 'ज'], ['z', 'झ'], ['T', 'ट'], ['D', 'ड'], ['N', 'ण'],
    ['t', 'त'], ['d', 'द'], ['n', 'न'], ['p', 'प'], ['f', 'फ'], ['b', 'ब'], ['m', 'म'],
    ['y', 'य'], ['r', 'र'], ['L', 'ळ'], ['l', 'ल'], ['v', 'व'], ['w', 'व'], ['s', 'स'], ['h', 'ह'],
    ['x', 'क्ष'], ['q', 'क'], ['c', 'क']
  ];
  var R_VOW = [
    ['aa', 'आ', 'ा'], ['ai', 'ऐ', 'ै'], ['au', 'औ', 'ौ'], ['ou', 'औ', 'ौ'], ['ee', 'ई', 'ी'], ['ii', 'ई', 'ी'],
    ['oo', 'ऊ', 'ू'], ['uu', 'ऊ', 'ू'], ['a', 'अ', ''], ['i', 'इ', 'ि'], ['u', 'उ', 'ु'],
    ['e', 'ए', 'े'], ['o', 'ओ', 'ो'], ['A', 'आ', 'ा'], ['I', 'ई', 'ी'], ['U', 'ऊ', 'ू'], ['E', 'ऐ', 'ै'], ['O', 'औ', 'ौ']
  ];
  function matchAt(s, i, table) {
    for (var k = 0; k < table.length; k++) {
      var t = table[k][0];
      if (s.substr(i, t.length) === t) return table[k];
    }
    return null;
  }
  function rulesToDev(input) {
    var s = input.replace(/[^A-Za-z]/g, '');
    if (!s) return '';
    // a capital first letter is usually just auto-capitalisation
    s = s.charAt(0).toLowerCase() + s.slice(1);
    var toks = [], i = 0;
    while (i < s.length) {
      var v = matchAt(s, i, R_VOW);
      var c = v ? null : matchAt(s, i, R_CONS);
      if (!v && !c) { var lc = s.charAt(i).toLowerCase(); v = matchAt(lc, 0, R_VOW); c = v ? null : matchAt(lc, 0, R_CONS); }
      if (v) { toks.push({ t: 'v', ind: v[1], mat: v[2], r: v[0].toLowerCase() }); i += v[0].length; }
      else if (c) { toks.push({ t: 'c', d: c[1], r: c[0].toLowerCase() }); i += c[0].length; }
      else i++;
    }
    var out = '';
    for (var k = 0; k < toks.length; k++) {
      var tk = toks[k], prev = toks[k - 1], next = toks[k + 1];
      if (tk.t === 'v') {
        var last = k === toks.length - 1;
        if (prev && prev.t === 'c') {
          var m = tk.mat;
          if (last && tk.r === 'a' && toks.length > 2) m = 'ा';      // kasa -> कसा
          else if (last && tk.r === 'i') m = 'ी';                     // pani -> पाणी
          else if (last && tk.r === 'u') m = 'ू';                     // tu -> तू
          out += m;
        } else {
          var ind = tk.ind;
          if (last && tk.r === 'i' && prev) ind = 'ई';
          out += ind;
        }
      } else {
        // n / m before another consonant -> anusvara (आनंद, संपूर्ण)
        if ((tk.r === 'n' || tk.r === 'm') && prev && prev.t === 'v' && next && next.t === 'c' &&
            !/^(n|m|y|r|v|w|h)$/.test(next.r) && !(tk.r === 'm' && next.r !== 'p' && next.r !== 'b' && next.r !== 'bh' && next.r !== 'ph')) {
          out += 'ं';
          continue;
        }
        out += tk.d;
        if (next && next.t === 'c') out += '्';
      }
    }
    return out;
  }

  // ---------------------------------------------------------------------
  // Chat shorthand that no dictionary romanisation will reach
  // ---------------------------------------------------------------------
  var SHORTHAND = dict({
    'mla': 'मला', 'tla': 'तला', 'tula': 'तुला', 'mnje': 'म्हणजे', 'mhnje': 'म्हणजे', 'mhanje': 'म्हणजे',
    'kr': 'कर', 'krto': 'करतो', 'krte': 'करते', 'krtoy': 'करतोय', 'krtey': 'करतेय', 'krun': 'करून', 'kel': 'केलं',
    'pn': 'पण', 'nhi': 'नाही', 'nai': 'नाही', 'nay': 'नाय', 'ny': 'नाही', 'n': 'न', 'v': 'व',
    'ahe': 'आहे', 'ahes': 'आहेस', 'aahe': 'आहे', 'ahet': 'आहेत', 'hota': 'होता',
    'kay': 'काय', 'kai': 'काय', 'ky': 'काय', 'ka': 'का', 'kasa': 'कसा', 'kashi': 'कशी', 'kas': 'कसं',
    'kuthe': 'कुठे', 'kuthay': 'कुठाय', 'kdhi': 'कधी', 'kadhi': 'कधी', 'kiti': 'किती', 'kon': 'कोण',
    'mi': 'मी', 'tu': 'तू', 'to': 'तो', 'ti': 'ती', 'te': 'ते', 'amhi': 'आम्ही', 'tumhi': 'तुम्ही', 'aapan': 'आपण',
    'maza': 'माझा', 'mazi': 'माझी', 'maze': 'माझे', 'tuza': 'तुझा', 'tuzi': 'तुझी', 'tuze': 'तुझे',
    'mazya': 'माझ्या', 'tuzya': 'तुझ्या', 'tyacha': 'त्याचा', 'ticha': 'तिचा', 'tyancha': 'त्यांचा',
    'ho': 'हो', 'hoy': 'होय', 'nko': 'नको', 'nako': 'नको', 'bg': 'बघ', 'bagh': 'बघ', 'chal': 'चल',
    'ata': 'आता', 'atta': 'आत्ता', 'aaj': 'आज', 'udya': 'उद्या', 'kal': 'काल', 'ani': 'आणि', 'aani': 'आणि',
    'khup': 'खूप', 'jara': 'जरा', 'ekda': 'एकदा', 'parat': 'परत', 'bhari': 'भारी', 'mast': 'मस्त',
    'zala': 'झाला', 'zali': 'झाली', 'zale': 'झाले', 'zal': 'झालं', 'jevlas': 'जेवलास', 'jevlis': 'जेवलीस',
    'thik': 'ठीक', 'barobar': 'बरोबर', 'sagla': 'सगळा', 'sagle': 'सगळे', 'sagl': 'सगळं', 'kharach': 'खरंच',
    'bol': 'बोल', 'sang': 'सांग', 'ye': 'ये', 'ja': 'जा', 'yeto': 'येतो', 'yete': 'येते', 'jato': 'जातो', 'jate': 'जाते',
    'ghari': 'घरी', 'ghar': 'घर', 'ithe': 'इथे', 'tithe': 'तिथे', 'asa': 'असा', 'ase': 'असे', 'as': 'असं',
    'tr': 'तर', 'tar': 'तर', 'mg': 'मग', 'mag': 'मग', 'tya': 'त्या', 'ya': 'या', 'he': 'हे', 'ha': 'हा', 'hi': 'ही',
    'dada': 'दादा', 'tai': 'ताई', 'aai': 'आई', 'baba': 'बाबा', 'mitra': 'मित्र', 'shala': 'शाळा',
    'bhetu': 'भेटू', 'bhetuya': 'भेटूया', 'bolu': 'बोलू', 'karu': 'करू', 'karuya': 'करूया',
    'de': 'दे', 'ghe': 'घे', 'kar': 'कर', 'thamb': 'थांब', 'bas': 'बस', 'chala': 'चला'
  });


  // Common English words typed in English spelling, with their usual Marathi spelling
  var ENGLISH = dict();
  ('mobile मोबाईल|phone फोन|call कॉल|message मेसेज|whatsapp व्हॉट्सॲप|video व्हिडिओ|photo फोटो|status स्टेटस|' +
   'recharge रिचार्ज|charge चार्ज|charger चार्जर|battery बॅटरी|laptop लॅपटॉप|computer कॉम्प्युटर|internet इंटरनेट|' +
   'online ऑनलाइन|offline ऑफलाइन|password पासवर्ड|email ईमेल|link लिंक|app ॲप|school स्कूल|college कॉलेज|class क्लास|' +
   'exam एक्झाम|result रिझल्ट|marks मार्क्स|teacher टीचर|sir सर|madam मॅडम|office ऑफिस|meeting मीटिंग|boss बॉस|job जॉब|' +
   'salary सॅलरी|interview इंटरव्ह्यू|project प्रोजेक्ट|company कंपनी|team टीम|manager मॅनेजर|report रिपोर्ट|file फाईल|' +
   'bus बस|train ट्रेन|station स्टेशन|ticket तिकीट|auto ऑटो|car कार|bike बाईक|petrol पेट्रोल|road रोड|traffic ट्रॅफिक|' +
   'late लेट|time टाइम|sorry सॉरी|thanks थँक्स|thank थँक|please प्लीज|ok ओके|okay ओके|hello हॅलो|bye बाय|good गुड|' +
   'morning मॉर्निंग|night नाईट|birthday बर्थडे|party पार्टी|happy हॅपी|movie मूव्ही|hotel हॉटेल|table टेबल|doctor डॉक्टर|' +
   'hospital हॉस्पिटल|medical मेडिकल|tension टेन्शन|problem प्रॉब्लेम|plan प्लॅन|cancel कॅन्सल|confirm कन्फर्म|ready रेडी|' +
   'done डन|free फ्री|busy बिझी|room रूम|hostel हॉस्टेल|canteen कॅन्टीन|bank बँक|account अकाउंट|card कार्ड|cash कॅश|' +
   'order ऑर्डर|delivery डिलिव्हरी|shop शॉप|market मार्केट|science सायन्स|maths मॅथ्स|engineering इंजिनिअरिंग|' +
   'lecture लेक्चर|practical प्रॅक्टिकल|notes नोट्स|book बुक|pen पेन|week वीक|weekend वीकेंड|sunday संडे|monday मंडे|' +
   'cricket क्रिकेट|match मॅच|game गेम|news न्यूज|update अपडेट|stop स्टॉप|best बेस्ट|idea आयडिया|group ग्रुप|post पोस्ट|' +
   'share शेअर|like लाईक|comment कमेंट|selfie सेल्फी|camera कॅमेरा|light लाईट|full फुल|plus प्लस|cool कूल|set सेट|' +
   'ready रेडी|screen स्क्रीन|download डाउनलोड|upload अपलोड|website वेबसाईट|google गूगल|youtube यूट्यूब|instagram इन्स्टाग्राम')
    .split('|').forEach(function (p) { var i = p.indexOf(' '); ENGLISH[p.slice(0, i)] = p.slice(i + 1); });

  // ---------------------------------------------------------------------
  // Engine
  // ---------------------------------------------------------------------
  function Engine() {
    this.words = [];       // Devanagari
    this.logf = [];        // log10 frequency
    this.vars = [];        // romanisations (computed lazily)
    this.index = dict();   // key2 -> [word ids]
    this.learned = dict(); // lowercased input -> {word: count}   (explicit picks)
    this.bigram = dict();  // previous word -> {word: count}      (context learned from use)
    this.english = dict(); // common English words
    this.user = dict();    // my dictionary: shortcut -> [texts]
    this.uwords = []; this.ulogf = []; this.uvars = []; this.uindex = dict();
    this.opts = { keepEnglish: false };
    this.keys = null;      // sorted index keys, for completions
  }

  // Accepts either the raw list ("word<TAB>probability") or the compiled
  // lexicon ("word<TAB>log10freq<TAB>key key ..."), see tools/build-data.js.
  Engine.prototype.load = function (text) {
    var lines = text.split('\n');
    for (var i = 0; i < lines.length; i++) {
      var line = lines[i];
      if (!line || line.charAt(0) === '#') continue;
      var f = line.split('\t');
      if (f.length >= 3) this.addCompiled(f[0], parseFloat(f[1]), f[2].split(' '));
      else this.addWord(f[0], f.length > 1 ? parseFloat(f[1]) : 1e-9);
    }
    this.keys = null;
    return this;
  };

  // one lowercase English word per line
  Engine.prototype.loadEnglish = function (text) {
    var lines = text.split('\n');
    for (var i = 0; i < lines.length; i++) { var w = lines[i].trim(); if (w && w.charAt(0) !== '#') this.english[w] = 1; }
    return this;
  };

  Engine.prototype.setOptions = function (o) {
    if (o && typeof o.keepEnglish === 'boolean') this.opts.keepEnglish = o.keepEnglish;
  };

  function addToIndex(index, keys, id) {
    for (var j = 0; j < keys.length; j++) {
      var k = keys[j];
      if (!k) continue;
      var b = index[k];
      if (b) { if (b.length < 400) b.push(id); } else index[k] = [id];
    }
  }

  Engine.prototype.addCompiled = function (w, logf, keys) {
    var id = this.words.length;
    this.words.push(w);
    this.logf.push(logf);
    this.vars.push(null);
    addToIndex(this.index, keys, id);
  };

  Engine.prototype.addWord = function (w, p) {
    var keys = wordKeys(w);
    if (!keys.length) return;
    this.addCompiled(w, Math.round(Math.log(p) / Math.LN10 * 100) / 100, keys);
  };

  function wordKeys(w) {
    var vs = devToRomanVariants(w), keys = [], seen = dict();
    for (var j = 0; j < vs.length; j++) {
      var k = key2(vs[j]);
      if (k && !seen[k]) { seen[k] = 1; keys.push(k); }
    }
    return keys;
  }

  var DEVA_WORD = /^[ऀ-ॿ‌‍]+$/;

  // "My dictionary": one entry per line.
  //   shortcut = text      typing the shortcut offers the text (any length, any script)
  //   देवनागरी              a word of your own that loose spellings should find
  // Lines starting with # are comments.
  Engine.prototype.setUserDict = function (text) {
    this.user = dict(); this.uwords = []; this.ulogf = []; this.uvars = []; this.uindex = dict();
    var seenW = dict(), self = this;
    function addU(w) {
      if (!DEVA_WORD.test(w) || seenW[w]) return;
      var keys = wordKeys(w); if (!keys.length) return;
      seenW[w] = 1;
      var id = self.uwords.length;
      self.uwords.push(w); self.ulogf.push(-2.5); self.uvars.push(null);
      addToIndex(self.uindex, keys, id);
    }
    var lines = String(text || '').split('\n'), n = 0;
    for (var i = 0; i < lines.length; i++) {
      var line = lines[i].replace(/\r$/, '').trim();
      if (!line || line.charAt(0) === '#') continue;
      var m = line.indexOf('='); if (m < 0) m = line.indexOf('\t');
      if (m > 0) {
        var sc = line.slice(0, m).replace(/[^A-Za-z]/g, '').toLowerCase(), val = line.slice(m + 1).trim();
        if (!sc || !val) continue;
        (this.user[sc] || (this.user[sc] = [])).push(val);
        addU(val); n++;
      } else { addU(line); n++; }
    }
    return n;
  };

  Engine.prototype.learn = function (input, word) {
    var k = input.replace(/[^A-Za-z]/g, '').toLowerCase();
    if (!k || !word) return;
    var m = this.learned[k] || (this.learned[k] = dict());
    m[word] = (m[word] || 0) + 1;
  };

  // remember that `word` followed `prev` (called for every committed word)
  Engine.prototype.learnContext = function (prev, word) {
    if (!prev || !word) return;
    var m = this.bigram[prev];
    if (!m) { if (Object.keys(this.bigram).length >= 20000) return; m = this.bigram[prev] = dict(); }
    m[word] = (m[word] || 0) + 1;
  };

  Engine.prototype.exportLearned = function () { return JSON.stringify({ v: 2, picks: this.learned, context: this.bigram }); };
  Engine.prototype.importLearned = function (json) {
    function nested(o) {
      var L = dict();
      if (o && typeof o === 'object') for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k) && o[k] && typeof o[k] === 'object') L[k] = dict(o[k]);
      return L;
    }
    try {
      var o = typeof json === 'string' ? JSON.parse(json) : json;
      if (o && o.v === 2) { this.learned = nested(o.picks); this.bigram = nested(o.context); }
      else { this.learned = nested(o); this.bigram = dict(); }
    } catch (e) { }
  };

  var W = { exact: 0.1, key1: 1.0, dist: 1.0, split: 2.0, english: 2.0, ctx: 1.5 };

  function cmpScored(x, y) { return y.s - x.s || (x.w < y.w ? -1 : x.w > y.w ? 1 : 0); }

  // English spellings of loanwords: phone -> fon, college -> colej, station -> steshan
  function englishForm(low) {
    var e = low.replace(/tion/g, 'shan').replace(/ture$/, 'char').replace(/ge$/, 'j').replace(/ce$/, 's');
    if (/[^aeiou][aeiou][^aeiouy]e$/.test(e)) e = e.slice(0, -1);
    return e === low ? null : e;
  }

  // Lexicon words for a lowercase Roman string, best first: [{w, s}]
  Engine.prototype.rank = function (low) {
    var scored = this.rankKey(low, 0);
    var en = englishForm(low);
    if (en) { scored = scored.concat(this.rankKey(en, W.english)); scored.sort(cmpScored); }
    return scored;
  };

  function scoreBucket(out, bucket, words, logf, vars, low, k1, penalty) {
    for (var b = 0; b < bucket.length; b++) {
      var id = bucket[b], vs = vars[id];
      if (!vs) vs = vars[id] = devToRomanVariants(words[id]);
      var best = -99;
      for (var j = 0; j < vs.length; j++) {
        var v = vs[j], s = 0;
        if (v === low) s += W.exact;
        var vk1 = key1(v);
        if (vk1 === k1) s += W.key1;
        s -= W.dist * editDist(k1, vk1);
        if (s > best) best = s;
      }
      out.push({ w: words[id], s: logf[id] + best - penalty });
    }
  }

  Engine.prototype.rankKey = function (low, penalty) {
    var k1 = key1(low), k2 = key2(low);
    var scored = [];
    scoreBucket(scored, this.index[k2] || [], this.words, this.logf, this.vars, low, k1, penalty);
    scoreBucket(scored, this.uindex[k2] || [], this.uwords, this.ulogf, this.uvars, low, k1, penalty);
    scored.sort(cmpScored);
    return scored;
  };

  // Long words that are not in the lexicon are often compounds or a word plus
  // a suffix-word: try splitting into two known words.
  Engine.prototype.compound = function (low) {
    if (low.length < 7) return null;
    var best = null;
    for (var i = 3; i <= low.length - 3; i++) {
      var a = this.rank(low.slice(0, i));
      if (!a.length) continue;
      var b = this.rank(low.slice(i));
      if (!b.length) continue;
      var s = a[0].s + b[0].s - W.split;
      if (!best || s > best.s) best = { w: a[0].w + b[0].w, s: s };
    }
    return best;
  };

  Engine.prototype.isEnglish = function (input) { return !!this.english[input.replace(/[^A-Za-z]/g, '').toLowerCase()]; };

  // Devanagari suggestions for a Roman word, best first.
  // prev: the word typed just before (Devanagari), used for context.
  Engine.prototype.suggest = function (input, max, prev) {
    max = max || 6;
    var raw = input.replace(/[^A-Za-z]/g, '');
    if (!raw) return [];
    var low = raw.toLowerCase();
    var scored = this.rank(low);
    var comp = this.compound(low);
    if (comp) scored.push(comp);
    // Capital T D N L (after the first letter) ask for ट ड ण ळ
    var want = [], caps = dict({ T: 'ट', D: 'ड', N: 'ण', L: 'ळ' });
    for (var ci = 1; ci < raw.length; ci++) if (caps[raw.charAt(ci)]) want.push(caps[raw.charAt(ci)]);
    if (want.length) {
      for (var si = 0; si < scored.length; si++)
        for (var wi = 0; wi < want.length; wi++) scored[si].s += scored[si].w.indexOf(want[wi]) >= 0 ? 1.5 : -4;
    }
    // context: words that followed `prev` before get a boost
    var bg = prev ? this.bigram[prev] : null;
    if (bg) for (var bi = 0; bi < scored.length; bi++) { var c0 = bg[scored[bi].w]; if (c0) scored[bi].s += W.ctx * Math.min(c0, 5); }
    scored.sort(cmpScored);

    var out = [], seen = dict();
    function push(w) { if (w && !seen[w]) { seen[w] = 1; out.push(w); } }

    // 1. my dictionary, then words this user picked before for exactly this input
    var ud = this.user[low];
    if (ud) for (var u = 0; u < ud.length; u++) push(ud[u]);
    var lm = this.learned[low];
    if (lm) {
      var ls = Object.keys(lm).sort(function (x, y) { return lm[y] - lm[x] || (x < y ? -1 : 1); });
      for (var l = 0; l < ls.length; l++) push(ls[l]);
    }
    var eng = !want.length && !!this.english[low];
    if (eng && this.opts.keepEnglish) push(raw);
    // 2. chat shorthand and English loanwords
    if (!want.length && SHORTHAND[low]) push(SHORTHAND[low]);
    if (!want.length && ENGLISH[low]) push(ENGLISH[low]);
    // 3. lexicon (+ compound)
    for (var c = 0; c < scored.length && out.length < max - 1; c++) push(scored[c].w);
    // 4. rule-based spelling (always offered so any word can be typed)
    push(rulesToDev(raw));
    out = out.slice(0, max);
    // a common English word: offer it unchanged as the 2nd choice
    if (eng && !seen[raw] && max > 1) { out.splice(Math.min(1, out.length), 0, raw); out = out.slice(0, max); }
    return out;
  };

  // Longer words that start like the input ("mahar" -> महाराष्ट्र), most frequent first.
  Engine.prototype.complete = function (input, n, exclude) {
    var raw = input.replace(/[^A-Za-z]/g, '');
    if (raw.length < 3 || !n) return [];
    var k2 = key2(raw.toLowerCase());
    if (k2.length < 2) return [];
    if (!this.keys) this.keys = Object.keys(this.index).sort();
    var keys = this.keys, lo = 0, hi = keys.length;
    while (lo < hi) { var mid = (lo + hi) >> 1; if (keys[mid] < k2) lo = mid + 1; else hi = mid; }
    var cand = [], seen = dict();
    if (exclude) for (var e = 0; e < exclude.length; e++) seen[exclude[e]] = 1;
    for (var i = lo; i < keys.length && i < lo + 4000; i++) {
      var k = keys[i];
      if (k.slice(0, k2.length) !== k2) break;
      if (k.length < k2.length + 1) continue;
      var b = this.index[k];
      for (var j = 0; j < b.length; j++) {
        var id = b[j];
        if (this.logf[id] < -5.5 || seen[this.words[id]]) continue;
        seen[this.words[id]] = 1;
        cand.push({ w: this.words[id], s: this.logf[id] });
      }
    }
    cand.sort(cmpScored);
    var out = [];
    for (var c = 0; c < cand.length && out.length < n; c++) out.push(cand[c].w);
    return out;
  };

  return {
    Engine: Engine,
    weights: W,
    rulesToDev: rulesToDev,
    devToRomanVariants: devToRomanVariants,
    wordKeys: wordKeys,
    editDist: editDist,
    key1: key1,
    key2: key2
  };
});
