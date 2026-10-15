//#region node_modules/.pnpm/preact@11.0.1/node_modules/preact/dist/preact.mjs
var e, t, n, r, i, a, o, s, c, l, u = {}, d = [], f = /^m(i|n|o|s|text|space)$/, p = Array.isArray, m = d.slice, h = Object.assign;
function g(e) {
	e && e.parentNode && e.remove();
}
function _(e, t, n) {
	var r, i, a, o = {}, s = arguments.length;
	for (a in t) a == "key" ? r = t[a] : a == "ref" && typeof e != "function" ? i = t[a] : o[a] = t[a];
	return s > 2 && (o.children = s > 3 ? m.call(arguments, 2) : n), v(e, o, r, i, null);
}
function v(n, r, i, a, o) {
	var s = {
		type: n,
		props: r,
		key: i,
		ref: a,
		__k: null,
		__: null,
		__b: 0,
		__e: null,
		__c: null,
		constructor: void 0,
		__v: o || ++t,
		__i: -1,
		__u: 0
	};
	return !o && e.vnode && e.vnode(s), s;
}
function y(e) {
	return e.children;
}
function b(e, t) {
	this.props = e, this.context = t, this.__g = 0;
}
function x(e, t) {
	if (t == null) return e.__ ? x(e.__, e.__i + 1) : null;
	for (var n; t < e.__k.length; t++) if ((n = e.__k[t]) && n.__e) return n.__e;
	return typeof e.type != "function" || e.props.__P ? null : x(e);
}
function S(e) {
	if ((e = e.__) && e.__c && !e.props.__P) return e.__e = null, e.__k.some(function(t) {
		return t && (e.__e = t.__e);
	}), S(e);
}
function C(t) {
	(8 & t.__g || !(t.__g |= 8) || !n.push(t) || i++) && r == e.debounceRendering || ((r = e.debounceRendering) || queueMicrotask)(w);
}
function w() {
	var t, r, o, s, c, l, u, d, f;
	try {
		for (r = 1; n.length;) n.length > r && n.sort(a), t = n.shift(), r = n.length, 8 & t.__g && (s = void 0, c = void 0, l = (c = (o = t).__v).__e, u = [], d = [], (f = o.__P) && ((s = h({ constructor: void 0 }, c)).__v = c.__v + 1, e.vnode && e.vnode(s), M(f, s, c, o.__n, f.namespaceURI, 32 & c.__u ? [l] : null, u, l || x(c), 32 & c.__u, d), s.__v = c.__v, s.__.__k[s.__i] = s, N(u, s, d), c.__ = c.__e = null, s.__e != l && S(s)));
	} finally {
		n.length = i = 0;
	}
}
function T(e, t, n, r, i, a, o, s, c, l, f) {
	var p, m, h, g, _, v, y = r.__k || d, b = t.length;
	for (c = E(n, t, y, c, b), p = 0; p < b; p++) (h = n.__k[p]) != null && (m = ~h.__i && y[h.__i] || u, h.__i = p, v = M(e, h, m, i, a, o, s, c, l, f), g = h.__e, (m.ref != h.ref || 8 & m.__u) && (m.ref && P(m.ref, null, h, m), h.ref && f.push(h.ref, h.__c || g, h)), _ = _ || g, 4 & h.__u ? (c = D(h, c, e, !m.__v), m.__e && (m.__e = null)) : typeof h.type == "function" && v !== void 0 ? c = v : g && (c = g.nextSibling), h.__u &= -7);
	return n.__e = _, c;
}
function E(e, t, n, r, i) {
	var a, o, s, c, l, u, d, f, m, h, g = n.length, _ = g, b = 0, S = !1, C = e.__k = Array(i);
	for (a = 0; a < i; a++) (o = t[a]) != null && typeof o != "boolean" && typeof o != "function" ? (typeof o != "object" || o.constructor == String ? o = C[a] = v(null, o) : p(o) ? o = C[a] = v(y, { children: o }) : o.constructor === void 0 && o.__b ? o = C[a] = v(o.type, o.props, o.key, o.ref, o.__v) : C[a] = o, c = a + b, o.__ = e, o.__b = e.__b + 1, s = null, ~(l = o.__i = O(o, n, c, _)) && (_--, (s = n[l]) && (s.__u |= 2)), s && s.__v ? (o.__u |= 2, l == c - 1 ? b-- : l == c + 1 ? b++ : l != c && (l > c ? b-- : b++, S = !0)) : (~l || (i > g ? b-- : i < g && b++), typeof o.type != "function" && (o.__u |= 4))) : C[a] = null;
	if (S) {
		for (u = [], d = [], a = 0; a < i; a++) if ((o = C[a]) && 2 & o.__u) {
			for (f = 0, m = u.length; f < m;) u[h = f + m >> 1] < o.__i ? f = h + 1 : m = h;
			u[f] = o.__i, d[a] = f + 1;
		}
		for (b = u.length; a--;) d[a] && (d[a] == b ? b-- : C[a].__u |= 4);
	}
	if (_) for (a = 0; a < g; a++) !(s = n[a]) || 2 & s.__u || (s.__e == r && (r = x(s)), F(s, s));
	return r;
}
function D(e, t, n, r) {
	var i, a, o;
	if (typeof e.type == "function") {
		if (e.props.__P) return t;
		if (i = e.__k) for (a = 0; a < i.length; a++) i[a] && (i[a].__ = e, t = D(i[a], t, n, !1));
		return t;
	}
	for (t && !t.parentNode && (t = x(e)) && !t.parentNode && (t = null), o = t; o && o.nodeType == 8;) o = o.nextSibling;
	for (e.__e != o && (!r && n.moveBefore && e.__e.parentNode ? n.moveBefore(e.__e, t) : n.insertBefore(e.__e, t || null)), t = e.__e; (t = t && t.nextSibling) && t.nodeType == 8;);
	return t;
}
function O(e, t, n, r) {
	var i, a, o, s = e.key, c = e.type, l = t[n], u = l && !(2 & l.__u);
	if (l === null && s == null || u && s == l.key && c == l.type) return n;
	if (r > +!!u) {
		for (i = n - 1, a = n + 1; i >= 0 || a < t.length;) if ((l = t[o = i >= 0 ? i-- : a++]) && !(2 & l.__u) && s == l.key && c == l.type) return o;
	}
	return -1;
}
function k(e, t, n) {
	n == null && (n = ""), t[0] == "-" ? e.setProperty(t, n) : e[t] = n;
}
function A(e, t, n, r, i) {
	var a, o;
	n: if (t == "style") {
		if (typeof n == "string") e.style.cssText = n;
		else {
			if (typeof r == "string" && (e.style.cssText = r = ""), r) for (t in r) n && t in n || k(e.style, t, "");
			if (n) for (t in n) r && n[t] == r[t] || k(e.style, t, n[t]);
		}
	} else if (t[0] == "o" && t[1] == "n") (e.__e || (e.__e = {}))[t] = n, n && r || (o = l[t] || (l[t] = j(t)), (e.__a || (e.__a = {}))[t] = c, a = t != (t = t.replace(s, "$1")), (t = t.slice(2))[0] < "a" && (t = t.toLowerCase()), n ? e.addEventListener(t, o, a) : e.removeEventListener(t, o, a));
	else {
		if (i == "http://www.w3.org/2000/svg") t = t.replace(/xlink(H|:h)/, "h").replace(/sName$/, "s");
		else if (t != "width" && t != "height" && t != "href" && t != "list" && t != "form" && t != "tabIndex" && t != "download" && t != "rowSpan" && t != "colSpan" && t != "role" && t != "popover" && t in e) try {
			e[t] = n == null ? "" : n;
			break n;
		} catch {}
		typeof n == "function" || (n == null || !1 === n && t[4] != "-" ? e.removeAttribute(t) : e.setAttribute(t, t == "popover" && n == 1 ? "" : n));
	}
}
function j(t) {
	return function(n) {
		if (this.__e) {
			var r = this.__e[t];
			if (n[o] == null) n[o] = c++;
			else if (n[o] < this.__a[t]) return;
			return r(e.event ? e.event(n) : n);
		}
	};
}
function M(t, n, r, i, a, o, s, c, l, u) {
	var f, m, _, v, S, C, w, E, D, O, k, A, j, M, N, P, I, re, L, R, z = n.type;
	if (n.constructor !== void 0) return null;
	if (128 & r.__u && (l = 32 & r.__u, f = r.__c.__z)) {
		if (n.__u |= l, m = o = [], f.nodeType == 8) for (_ = 1, v = f.nextSibling; v; v = v.nextSibling) {
			if (v.nodeType == 8) {
				if (v.data.startsWith("$s")) _++;
				else if (v.data.startsWith("/$s") && !--_) break;
			}
			o.push(v);
		}
		else o.push(f);
		c = o[0];
	}
	(f = e.__b) && f(n);
	n: if (typeof z == "function") {
		S = s.length;
		try {
			if (O = n.props, k = (f = z.prototype) && f.render, A = (f = z.contextType) && i[f.__c], j = f ? A ? A.props.value : f.__ : i, r.__c ? 2 & (C = n.__c = r.__c).__g && (C.__g |= 1) : (k ? n.__c = C = new z(O, j) : (n.__c = C = new b(O, j), C.constructor = z, C.render = ne), A && A.sub(C), C.state || (C.state = {}), C.__n = i, C.__g |= 8, C.__h = [], C.__k = []), k && (C.__s || (C.__s = C.state), z.getDerivedStateFromProps && (C.__s == C.state && (C.__s = h({}, C.__s)), h(C.__s, z.getDerivedStateFromProps(O, C.__s)))), w = C.props, E = C.state, C.__v = n, r.__c) {
				if (k && !z.getDerivedStateFromProps && O !== w && C.componentWillReceiveProps && C.componentWillReceiveProps(O, j), n.__v == r.__v && !(8 & C.__g) || !(4 & C.__g) && C.shouldComponentUpdate && !1 === C.shouldComponentUpdate(O, C.__s, j)) {
					n.__v != r.__v && (C.props = O, C.state = C.__s, C.__g &= -9), n.__e = r.__e, n.__k = r.__k, n.__k.some(function(e) {
						e && (e.__ = n);
					}), d.push.apply(C.__h, C.__k), C.__k = [], C.__h.length && s.push(C), c = x(r);
					break n;
				}
				C.componentWillUpdate && C.componentWillUpdate(O, C.__s, j), k && C.componentDidUpdate && C.__h.push(function() {
					C.componentDidUpdate(w, E, D);
				});
			} else k && !z.getDerivedStateFromProps && C.componentWillMount && C.componentWillMount(), k && C.componentDidMount && C.__h.push(C.componentDidMount);
			if (C.context = j, C.props = O, C.__P = t, C.__g &= -5, M = e.__r, N = 0, k) C.state = C.__s, C.__g &= -9, M && M(n), f = C.render(C.props, C.state, C.context), d.push.apply(C.__h, C.__k), C.__k = [];
			else do
				C.__g &= -9, M && M(n), f = C.render(C.props, C.state, C.context), C.state = C.__s;
			while (8 & C.__g && ++N < 25);
			C.state = C.__s, C.getChildContext && (i = h({}, i, C.getChildContext())), k && r.__c && C.getSnapshotBeforeUpdate && (D = C.getSnapshotBeforeUpdate(w, E)), P = f && f.type === y && f.key == null ? f.props.children : f, O.__P && (f = c, a = (t = O.__P).namespaceURI, l = o = null, r.props && r.props.__P != t && (r.__k.some(function(e) {
				e && F(e, e);
			}), r.__k = null), c = r.__k ? x(r, 0) : null), c = T(t, p(P) ? P : [P], n, r, i, a, o, s, c, l, u), O.__P && (n.__e = null, c = f), n.__u &= -161, 128 & r.__u && (C.__z = null), m && m.some(g), C.__h.length && s.push(C), 1 & C.__g && (C.__g &= -4);
		} catch (t) {
			if (s.length = S, n.__v = null, l || o) {
				if (t.then) {
					if (I = 0, n.__u |= l ? 160 : 128, o) {
						for (~(L = o.indexOf(c || void 0)) || (L = o.length); (R = o[L - 1]) && R.nodeType == 8;) L--;
						for (; L < o.length; L++) if (R = o[L]) {
							if (o[L] = null, R.nodeType == 8) {
								if (R.data.startsWith("$s")) I++ || (re = R);
								else if (I && R.data.startsWith("/$s") && !--I) {
									c = R;
									break;
								}
							} else if (!I) break;
						}
					}
					if (!re) {
						for (; c && c.nodeType == 8 && c.nextSibling;) c = c.nextSibling;
						re = c;
					}
					n.__c.__z || (n.__c.__z = re), n.__e = c;
				} else o && o.some(g);
			} else n.__e = r.__e;
			n.__k || (n.__k = r.__k || []), t.then || ee(n), e.__e(t, n, r);
		}
	} else c = n.__e = te(r.__e, n, r, i, a, o, s, l, u, t);
	return (f = e.diffed) && f(n), 128 & n.__u ? void 0 : c;
}
function ee(e) {
	e && (e.__c && (e.__c.__g |= 4), e.__k && e.__k.some(ee));
}
function N(t, n, r) {
	for (var i = 0; i < r.length;) P(r[i++], r[i++], r[i++]);
	e.__c && e.__c(n, t), t.some(function(n) {
		try {
			t = n.__h, n.__h = [], t.some(function(e) {
				e.call(n);
			});
		} catch (t) {
			e.__e(t, n.__v);
		}
	});
}
function te(t, n, r, i, a, o, s, c, l, d) {
	var h, _, v, y, b, S, C, w, E, D = r.props || u, O = n.props, k = n.type;
	if (k == "svg" ? a = "http://www.w3.org/2000/svg" : k == "math" ? a = "http://www.w3.org/1998/Math/MathML" : a || (a = "http://www.w3.org/1999/xhtml"), o) {
		for (h = 0; h < o.length; h++) if ((b = o[h]) && (k ? b.localName == k : b.nodeType == 3)) {
			t = b, o[h] = null;
			break;
		}
	}
	if (!t) {
		if (w = d.ownerDocument || document, !k) return w.createTextNode(O);
		t = w.createElementNS(a, k, O.is && O), c && (e.__m && e.__m(n, o), c = !1), o = null;
	}
	if (k) {
		if (d = k == "template" ? t.content : t, o = k == "textarea" && O.defaultValue != null ? null : o && m.call(d.childNodes), !c && o) for (D = {}, h = 0; h < t.attributes.length; h++) D[(b = t.attributes[h]).name] = b.value;
		for (h in D) b = D[h], h == "dangerouslySetInnerHTML" ? v = b : h == "children" || h in O || h == "value" && "defaultValue" in O || h == "checked" && "defaultChecked" in O || A(t, h, null, b, a);
		for (h in E = 1 & r.__u, O) b = O[h], h == "children" ? y = b : h == "dangerouslySetInnerHTML" ? _ = b : h == "value" ? S = b : h == "checked" ? C = b : c && typeof b != "function" || !(D[h] !== b || E && b != null) || A(t, h, b, D[h], a);
		_ ? (c || v && (_.__html == v.__html || _.__html == t.innerHTML) || (t.innerHTML = _.__html), n.__k = []) : (v && (t.textContent = ""), (k == "foreignObject" || a == "http://www.w3.org/1998/Math/MathML" && f.test(k)) && (a = "http://www.w3.org/1999/xhtml"), T(d, p(y) ? y : [y], n, r, i, a, o, s, o ? o[0] : r.__k && x(r, 0), c, l), o && o.some(g)), c && k != "textarea" || (h = "value", k == "progress" && S == null ? t.removeAttribute(h) : S == null || S === t[h] && (k != "progress" || S) || A(t, h, S, D[h], a), h = "checked", C != null && C != t[h] && A(t, h, C, D[h], a));
	} else D === O || c && t.data == O || (t.data = O);
	return t;
}
function P(t, n, r, i) {
	try {
		typeof t == "function" ? n ? n.__x = t(n) || 1 : i && (i = i.__c || i.__e) && (n = i.__x) && (i.__x = null, typeof n == "function" ? n() : t(null)) : t.current = n;
	} catch (t) {
		e.__e(t, r);
	}
}
function F(t, n, r) {
	var i, a;
	if (e.unmount && e.unmount(t), !(i = t.ref) || i.current && i.current != t.__e || P(i, null, n, t), i = t.__c) {
		if (i.componentWillUnmount) try {
			i.componentWillUnmount();
		} catch (t) {
			e.__e(t, n);
		}
		i.__P = i.__n = null;
	}
	if (i = t.__k) for (a = 0; a < i.length; a++) i[a] && F(i[a], n, typeof t.type != "function" || r && !t.props.__P);
	(i = t.__e) && (r || g(i), i.__e && (i.__e = null)), t.__e = t.__c = t.__ = null;
}
function ne(e, t, n) {
	return this.constructor(e, n);
}
function I(t, n) {
	var r, i, a, o;
	e.__ && e.__(t, n), n.nodeType == 9 && (n = n.documentElement), i = (r = t && 32 & t.__u) ? null : n.__k, n.__k = v(y, { children: [t] }), a = [], o = [], M(n, n.__k, i || u, u, n.namespaceURI, i ? null : n.firstChild ? m.call(n.childNodes) : null, a, i ? i.__e : n.firstChild, r, o), N(a, n.__k, o), n.__k.props.children = null;
}
e = { __e: function(e, t, n, r) {
	for (var a, o, s; t = t.__;) if ((a = t.__c) && !(1 & a.__g)) {
		a.__g |= 4;
		try {
			if ((o = a.constructor) && o.getDerivedStateFromError && (a.setState(o.getDerivedStateFromError(e)), s = 8 & a.__g), a.componentDidCatch && (a.componentDidCatch(e, r || {}), s = 8 & a.__g), s) return void (a.__g |= 2);
		} catch (t) {
			e = t, s = 0;
		}
	}
	throw i = 0, e;
} }, t = 0, b.prototype.setState = function(e, t) {
	var n = this.__s;
	n && n != this.state || (n = this.__s = h({}, this.state)), typeof e == "function" && (e = e(h({}, n), this.props)), e && (h(n, e), this.__v && (t && this.__k.push(t), C(this)));
}, b.prototype.forceUpdate = function(e) {
	this.__v && (this.__g |= 4, e && this.__h.push(e), C(this));
}, b.prototype.render = y, n = [], i = 0, a = function(e, t) {
	return e.__v.__b - t.__v.__b;
}, o = Symbol(), s = /(PointerCapture)$|Capture$/i, c = 0, l = {};
//#endregion
//#region widget/src/html.ts
var re = /* @__PURE__ */ new Set([
	"P",
	"BR",
	"A",
	"STRONG",
	"B",
	"EM",
	"I",
	"U",
	"S",
	"UL",
	"OL",
	"LI",
	"CODE",
	"PRE",
	"BLOCKQUOTE",
	"SPAN",
	"DIV",
	"H2",
	"H3",
	"HR"
]), L = /* @__PURE__ */ new Set([
	"SCRIPT",
	"STYLE",
	"IFRAME",
	"OBJECT",
	"EMBED",
	"TEMPLATE",
	"NOSCRIPT",
	"SVG",
	"MATH",
	"FORM",
	"INPUT",
	"BUTTON",
	"TEXTAREA",
	"SELECT",
	"LINK",
	"META",
	"IMG",
	"VIDEO",
	"AUDIO"
]);
function R(e, t) {
	var n;
	if (e.nodeType === 3) return t.createTextNode((n = e.textContent) == null ? "" : n);
	if (e.nodeType !== 1) return null;
	let r = e, i = r.tagName.toUpperCase();
	if (L.has(i)) return null;
	let a = Array.from(r.childNodes).map((e) => R(e, t)).filter((e) => e !== null);
	if (!re.has(i)) {
		let e = t.createDocumentFragment();
		return a.forEach((t) => e.appendChild(t)), e;
	}
	let o = t.createElement(i === "DIV" ? "p" : i.toLowerCase());
	if (i === "A") {
		var s;
		let e = (s = r.getAttribute("href")) == null ? "" : s;
		/^(https?:|mailto:)/i.test(e.trim()) && (o.setAttribute("href", e.trim()), o.setAttribute("target", "_blank"), o.setAttribute("rel", "noopener noreferrer nofollow"));
	}
	return a.forEach((e) => o.appendChild(e)), o;
}
function z(e) {
	if (!e) return "";
	let t = new DOMParser().parseFromString(`<body>${e}</body>`, "text/html"), n = document.implementation.createHTMLDocument(""), r = n.createElement("div");
	return Array.from(t.body.childNodes).forEach((e) => {
		let t = R(e, n);
		t && r.appendChild(t);
	}), r.innerHTML;
}
function ie(e) {
	let t = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(e.trim());
	if (!t) return "#ffffff";
	let n = t[1];
	n.length === 3 && (n = n.split("").map((e) => e + e).join(""));
	let [r, i, a] = [
		0,
		2,
		4
	].map((e) => parseInt(n.slice(e, e + 2), 16) / 255).map((e) => e <= .03928 ? e / 12.92 : ((e + .055) / 1.055) ** 2.4);
	return .2126 * r + .7152 * i + .0722 * a > .4 ? "#0a0a0a" : "#ffffff";
}
function ae(e) {
	return !!e && /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i.test(e.trim());
}
function oe(e) {
	return e < 1024 ? `${e} B` : e < 1048576 ? `${Math.round(e / 1024)} KB` : `${(e / 1048576).toFixed(1)} MB`;
}
function se(e) {
	let t = e.trim().split(/\s+/).filter(Boolean);
	return t.length ? (t.length === 1 ? t[0].slice(0, 2) : t[0][0] + t[t.length - 1][0]).toUpperCase() : "?";
}
function ce(e) {
	var t, n;
	if (!e || !/<ul/i.test(e)) return {
		html: e,
		sources: []
	};
	let r = new DOMParser().parseFromString(`<body>${e}</body>`, "text/html").body, i = r.lastElementChild;
	for (; r.lastChild && r.lastChild !== i && !((t = r.lastChild.textContent) == null ? "" : t).trim();) r.lastChild.remove();
	if (!i || i.tagName !== "UL" || r.lastChild !== i) return {
		html: e,
		sources: []
	};
	let a = Array.from(i.children), o = [];
	for (let t of a) {
		var s, c, l, u;
		let n = t.tagName === "LI" && t.children.length === 1 ? t.firstElementChild : null, r = (n == null ? void 0 : n.tagName) === "A" ? (s = n.getAttribute("href")) == null ? "" : s : "";
		if (!n || !/^https?:/i.test(r) || ((c = t.textContent) == null ? "" : c).trim() !== ((l = n.textContent) == null ? "" : l).trim()) return {
			html: e,
			sources: []
		};
		o.push({
			href: r,
			title: ((u = n.textContent) == null ? "" : u).trim() || r
		});
	}
	return i.remove(), !((n = r.textContent) == null ? "" : n).trim() || !o.length ? {
		html: e,
		sources: []
	} : {
		html: r.innerHTML,
		sources: o
	};
}
function le(e) {
	try {
		return new URL(e).hostname.replace(/^www\./, "");
	} catch {
		return "";
	}
}
//#endregion
//#region widget/src/config.ts
var ue = "Ollie", de = (e, t, n, r) => Math.min(n, Math.max(t, typeof e == "number" && Number.isFinite(e) ? Math.round(e) : r)), fe = (e, t) => typeof e == "string" ? e.trim().slice(0, t) : "", pe = (e, t, n) => t.includes(e) ? e : n, me = (e) => typeof e == "string" && /^https?:\/\//i.test(e.trim()) ? e.trim() : null;
function he(e) {
	var t, n, r;
	let i = e == null ? {} : e, a = Array.isArray(i.suggestions) ? i.suggestions.map((e) => fe(e, 60)).filter((e) => e.length >= 2).slice(0, 4) : [], o = Array.isArray(i.team) ? i.team.filter((e) => e && typeof e.name == "string").slice(0, 3).map((e) => ({
		name: e.name.trim().slice(0, 40),
		avatar_url: me(e.avatar_url)
	})) : [];
	return {
		workspace_name: fe(i.workspace_name, 80),
		brand_color: ae(i.brand_color) ? i.brand_color.trim() : null,
		greeting: typeof i.greeting == "string" ? i.greeting : "",
		help_center_url: me(i.help_center_url),
		require_email: i.require_email === !0,
		locale: (t = i.locale) == null ? null : t,
		avatar_url: me(i.avatar_url),
		launcher_icon_url: me(i.launcher_icon_url),
		theme: pe(i.theme, [
			"auto",
			"light",
			"dark"
		], "auto"),
		position: pe(i.position, ["right", "left"], "right"),
		offset_x: de(i.offset_x, 0, 200, 20),
		offset_y: de(i.offset_y, 0, 200, 20),
		launcher: pe(i.launcher, ["icon", "label"], "icon"),
		launcher_label: fe(i.launcher_label, 24),
		radius: pe(i.radius, [
			"square",
			"soft",
			"round"
		], "soft"),
		title: fe(i.title, 40),
		subtitle: fe(i.subtitle, 80),
		suggestions: a,
		sound: i.sound === !0,
		team: o,
		ai_answers: i.ai_answers === !0,
		bot: {
			name: fe((n = i.bot) == null ? void 0 : n.name, 30) || "Ollie",
			avatar_url: me((r = i.bot) == null ? void 0 : r.avatar_url)
		}
	};
}
var ge = {
	square: {
		panel: 6,
		bubble: 6,
		launcher: "10px",
		ctl: "4px",
		card: 4,
		bar: 4,
		av: "4px"
	},
	soft: {
		panel: 16,
		bubble: 14,
		launcher: "16px",
		ctl: "8px",
		card: 10,
		bar: 10,
		av: "6px"
	},
	round: {
		panel: 24,
		bubble: 20,
		launcher: "999px",
		ctl: "999px",
		card: 16,
		bar: 22,
		av: "999px"
	}
};
function _e(e) {
	var t, n, r;
	let i = ge[(t = e.radius) == null ? "soft" : t], a = {
		"--r-panel": `${i.panel}px`,
		"--r-bubble": `${i.bubble}px`,
		"--r-launcher": i.launcher,
		"--r-ctl": i.ctl,
		"--r-card": `${i.card}px`,
		"--r-bar": `${i.bar}px`,
		"--r-av": i.av,
		"--ox": `${(n = e.offset_x) == null ? 20 : n}px`,
		"--oy": `${(r = e.offset_y) == null ? 20 : r}px`
	};
	if (ae(e.brand_color)) {
		let t = e.brand_color.startsWith("#") ? e.brand_color : `#${e.brand_color}`;
		a["--brand"] = t, a["--brand-fg"] = ie(t);
	}
	return a;
}
function ve(e, t) {
	let n = _e(t);
	for (let t of ["--brand", "--brand-fg"]) t in n || e.style.removeProperty(t);
	for (let [t, r] of Object.entries(n)) e.style.setProperty(t, r);
}
function ye(e = typeof location > "u" ? void 0 : location.href) {
	if (!e) return;
	let t;
	try {
		t = new URL(e);
	} catch {
		return;
	}
	if (t.protocol !== "https:") return;
	t.hash = "", t.username = "", t.password = "";
	let n = t.href;
	return n.length > 500 && (n = (t.origin + t.pathname).slice(0, 500)), n;
}
//#endregion
//#region widget/src/api.ts
var be = class extends Error {
	constructor(e, t, n) {
		super(n), this.status = e, this.code = t;
	}
}, B = encodeURIComponent;
function xe(e) {
	var t;
	return e ? Array.isArray(e) ? e : (t = e.data) == null ? [] : t : [];
}
var Se = class {
	constructor(e, t, n) {
		this.base = e, this.key = t, this.fetcher = n, this.visitorToken = null;
	}
	async req(e, t, n, r = !0) {
		var i;
		let a = { Accept: "application/json" }, o;
		n instanceof FormData ? o = n : n !== void 0 && (a["Content-Type"] = "application/json", o = JSON.stringify(n)), r && this.visitorToken && (a.Authorization = `Visitor ${this.visitorToken}`);
		let s;
		try {
			s = await this.fetcher(this.base + t, {
				method: e,
				headers: a,
				body: o,
				credentials: "omit"
			});
		} catch {
			throw new be(0, "network_error", "Can't reach the server");
		}
		if (!s.ok) {
			let e = "error", t = `Request failed (${s.status})`;
			try {
				var c;
				let n = await s.json();
				e = (c = n.code) == null ? e : c, t = n.detail || n.title || t;
			} catch {}
			throw new be(s.status, e, t);
		}
		if (s.status !== 204 && ((i = s.headers.get("content-type")) == null ? "" : i).includes("json")) return await s.json();
	}
	config() {
		return this.req("GET", `/v1/widget/config?key=${encodeURIComponent(this.key)}`, void 0, !1);
	}
	async session(e = {}) {
		var t;
		let n = await this.req("POST", "/v1/widget/session", {
			key: this.key,
			visitor_token: (t = this.visitorToken) == null ? void 0 : t,
			...e
		}, !1);
		return this.visitorToken = n.visitor_token, n;
	}
	async conversations() {
		return xe(await this.req("GET", "/v1/widget/conversations"));
	}
	async start(e, t, n) {
		var r, i;
		let a = await this.req("POST", "/v1/widget/conversations", {
			body: e,
			attachment_ids: t,
			page_url: n
		});
		return {
			conversation: (r = a.conversation) == null ? a : r,
			message: (i = a.message) == null ? null : i
		};
	}
	async messages(e) {
		return xe(await this.req("GET", `/v1/widget/conversations/${B(e)}/messages`));
	}
	send(e, t, n, r) {
		return this.req("POST", `/v1/widget/conversations/${B(e)}/messages`, {
			body: t,
			attachment_ids: n,
			page_url: r
		});
	}
	handoff(e) {
		return this.req("POST", `/v1/widget/conversations/${B(e)}/handoff`, {});
	}
	decide(e, t, n) {
		return this.req("POST", `/v1/widget/conversations/${B(e)}/actions/${B(t)}`, { decision: n });
	}
	rate(e, t, n) {
		return this.req("POST", `/v1/widget/conversations/${B(e)}/messages/${B(t)}/rating`, { rating: n });
	}
	upload(e) {
		let t = new FormData();
		return t.append("file", e), this.req("POST", "/v1/widget/attachments", t);
	}
};
//#endregion
//#region widget/src/ai.ts
function Ce(e) {
	var t;
	return ((t = e.author) == null ? void 0 : t.type) === "system" && e.ai === !0;
}
function we(e) {
	return e.some((e) => {
		var t;
		return e.direction === "outbound" && ((t = e.author) == null ? void 0 : t.type) === "user";
	});
}
function Te(e) {
	return (e == null ? void 0 : e.ai_handoff) === !0;
}
function Ee(e) {
	return e.handedOff || Te(e.conversation) || e.conversation && (e.conversation.status === "solved" || e.conversation.status === "closed") || we(e.msgs) ? !1 : e.aiAnswers || e.msgs.some(Ce);
}
function De(e) {
	for (let t = e.length - 1; t >= 0; t--) if (Ce(e[t])) return e[t];
	return null;
}
function Oe(e, t) {
	var n;
	return e.status === "pending" ? (n = t == null ? void 0 : t.status) == null ? "pending" : n : e.status;
}
//#endregion
//#region widget/src/i18n.ts
var ke = {
	chatWith: "Chat with {name}",
	support: "Support",
	openChat: "Open chat",
	openChatUnread: "Open chat, {count} new",
	closeChat: "Close chat",
	greeting: "Hi there. How can we help?",
	replyTime: "We usually reply within a few hours.",
	unavailable: "Chat is unavailable",
	unavailableHint: "Check your connection and try again later.",
	sendMessage: "Send us a message",
	yourConversations: "Your conversations",
	conversation: "Conversation",
	newConversation: "New conversation",
	typing: "{name} is typing…",
	searchHelp: "Search the help center",
	poweredBy: "Powered by opendesk",
	back: "Back to conversations",
	emptyHint: "Ask us anything. We'll reply here, and by email if you leave one.",
	automated: "Automated",
	notSent: "Not sent.",
	retry: "Retry",
	sending: "Sending…",
	captureHint: "Leave your email so we can reply if you step away.",
	name: "Name",
	email: "Email",
	emailPlaceholder: "you@company.com",
	invalidEmail: "Enter a valid email address.",
	saveFailed: "Couldn't save that. Try again.",
	saving: "Saving…",
	continue: "Continue",
	dropFiles: "Drop files to attach",
	removeFile: "Remove {name}",
	placeholder: "Write a message…",
	placeholderEmail: "Add your email above to start",
	message: "Message",
	attach: "Attach files",
	send: "Send",
	newMessage: "New message from {name}: {text}",
	ai: "AI",
	aiAnswer: "Answer written by AI",
	sources: "Sources",
	talkToPerson: "Talk to a person",
	handoffDone: "We've let the team know. Someone will reply here.",
	handoffFailed: "Couldn't reach the team. Try again.",
	chatWithUs: "Chat with us",
	replyTimeAi: "{name} answers right away, the team within a few hours.",
	suggested: "Suggested",
	botTyping: "{name} is writing…",
	checking: "Checking {tool}…",
	checked: "Checked {tool}",
	toolFailed: "Couldn't check {tool}",
	handedOff: "A person will reply here",
	handedOffHint: "You'll see their reply in this conversation, and by email if you left one.",
	actionPending: "Needs your confirmation",
	confirm: "Confirm",
	cancel: "Cancel",
	actionConfirmed: "Confirmed",
	actionDeclined: "Cancelled",
	actionExpired: "Expired",
	actionFailed: "Didn't go through",
	actionError: "Couldn't send that. Try again.",
	rateUp: "Helpful",
	rateDown: "Not helpful",
	ratedThanks: "Thanks for the feedback",
	jumpLatest: "Jump to latest"
}, Ae = /* @__PURE__ */ new Set(["ar"]), V = "en", je = ke;
function Me() {
	return V;
}
function Ne(e = V) {
	return Ae.has(e);
}
function H(e, t) {
	let n = je[e] || ke[e];
	return t ? n.replace(/\{(\w+)\}/g, (e, n) => n in t ? String(t[n]) : e) : n;
}
function U() {
	return new Intl.RelativeTimeFormat(V, {
		numeric: "auto",
		style: "narrow"
	});
}
function Pe(e) {
	return e.charAt(0).toLocaleUpperCase(V) + e.slice(1);
}
function Fe(e, t = Date.now()) {
	let n = Math.round((t - Date.parse(e)) / 1e3);
	if (n < 45) return U().format(0, "second");
	let r = Math.round(n / 60);
	if (r < 60) return U().format(-r, "minute");
	let i = Math.round(r / 60);
	if (i < 24) return U().format(-i, "hour");
	let a = Math.round(i / 24);
	return a < 7 ? U().format(-a, "day") : new Date(e).toLocaleDateString(V, {
		month: "short",
		day: "numeric"
	});
}
function Ie(e) {
	return new Intl.DateTimeFormat(V, {
		hour: "numeric",
		minute: "2-digit"
	}).format(new Date(e));
}
function Le(e, t = /* @__PURE__ */ new Date()) {
	let n = new Date(e);
	if (n.toDateString() === t.toDateString()) return Pe(U().format(0, "day"));
	let r = /* @__PURE__ */ new Date(t.getTime() - 864e5);
	return n.toDateString() === r.toDateString() ? Pe(U().format(-1, "day")) : n.toLocaleDateString(V, {
		weekday: "short",
		month: "short",
		day: "numeric"
	});
}
//#endregion
//#region widget/src/live.ts
var Re = (e, t) => {
	if (!t.length) return e;
	let n = { ...e.replies };
	for (let e of t) delete n[e];
	return {
		replies: n,
		closed: [...e.closed.filter((e) => !t.includes(e)), ...t].slice(-50)
	};
};
function ze(e, t, n, r) {
	let i = t.data;
	if (typeof (i == null ? void 0 : i.reply_id) != "string" || typeof i.ticket_id != "string") return e;
	let a = i.reply_id;
	if (e.closed.includes(a)) return e;
	let o = e.replies[a], s = () => ({
		id: a,
		ticket_id: i.ticket_id,
		bot: null,
		text: "",
		tools: [],
		phase: "typing",
		partial: !0,
		message_id: null,
		handoff: !1,
		action_id: null,
		at: n
	}), c = (t) => ({
		...e,
		replies: {
			...e.replies,
			[a]: t
		}
	});
	if (o && (o.phase === "done" || o.phase === "leaving") && t.type !== "ai.reply.ended") return e;
	switch (t.type) {
		case "ai.reply.started": {
			var l;
			let e = typeof ((l = t.data.bot) == null ? void 0 : l.name) == "string" ? t.data.bot.name : null;
			return c(o ? {
				...o,
				bot: e == null ? o.bot : e,
				partial: o.text !== "" && o.partial,
				at: n
			} : {
				...s(),
				bot: e,
				partial: !1
			});
		}
		case "ai.reply.delta": {
			if (typeof t.data.text != "string" || !t.data.text) return e;
			let r = o == null ? s() : o;
			return c({
				...r,
				text: r.text + t.data.text,
				phase: "streaming",
				at: n
			});
		}
		case "ai.reply.tool": {
			let { tool: r, status: i } = t.data;
			if (typeof r != "string" || !r) return e;
			let a = i === "done" || i === "error" ? i : "running", l = o == null ? s() : o, u = [...l.tools], d = -1;
			for (let e = u.length - 1; e >= 0; e--) if (u[e].name === r && u[e].status === "running") {
				d = e;
				break;
			}
			return d >= 0 ? u[d] = {
				name: r,
				status: a
			} : u.push({
				name: r,
				status: a
			}), c({
				...l,
				tools: u,
				at: n
			});
		}
		case "ai.reply.ended": {
			let i = typeof t.data.message_id == "string" && t.data.message_id ? t.data.message_id : null;
			if (i && r(i)) return Re(e, [a]);
			let l = o == null ? s() : o, u = l.tools.map((e) => e.status === "running" ? {
				...e,
				status: "done"
			} : e);
			return c({
				...l,
				tools: u,
				phase: i ? "done" : "leaving",
				message_id: i,
				handoff: t.data.handoff === !0,
				action_id: typeof t.data.action_id == "string" ? t.data.action_id : null,
				at: n
			});
		}
		default: return e;
	}
}
function Be(e, t) {
	let n = [], r = !1;
	for (let i of Object.values(e.replies)) (i.message_id === t.id || t.ai === !0 && t.direction === "outbound" && i.ticket_id === t.ticket_id && (i.phase === "typing" || i.phase === "streaming")) && (n.push(i.id), r = !0);
	return {
		state: Re(e, n),
		replaced: r
	};
}
function Ve(e, t) {
	return "id" in t ? e.replies[t.id] ? Re(e, [t.id]) : e : Re(e, Object.values(e.replies).filter((e) => e.phase === "done" || e.phase === "leaving" ? e.at < t.doneBefore : e.at < t.idleBefore).map((e) => e.id));
}
function He(e, t) {
	return Object.values(e.replies).filter((e) => e.ticket_id === t);
}
//#endregion
//#region widget/src/markdown.ts
var Ue = /[ \t]*\[\d{1,3}(?:\s*,\s*\d{1,3})*\](?!\()/g, We = /[ \t]*\[\d{0,3}(?:\s*,\s*\d{0,3})*$/;
function Ge(e, t = !1) {
	let n = e.replace(Ue, "");
	return t && (n = n.replace(We, "")), n;
}
function Ke(e) {
	var t, n, r;
	let i = e;
	if ((((t = i.match(/```/g)) == null ? [] : t).length & 1) == 1) return i;
	if ((((n = i.match(/\*\*/g)) == null ? [] : n).length & 1) == 1) {
		let e = i.lastIndexOf("**");
		i = i.slice(0, e) + i.slice(e + 2);
	}
	if ((((r = i.replace(/```/g, "").match(/`/g)) == null ? [] : r).length & 1) == 1) {
		let e = i.lastIndexOf("`");
		i = i.slice(0, e) + i.slice(e + 1);
	}
	return i = i.replace(/\[([^\]\n]*)\]\([^)\s]*$/, "$1").replace(/\[([^\]\n]*)$/, "$1"), i;
}
var qe = /^(https?:\/\/|mailto:)/i, Je = /(\*\*([^*\n]+?)\*\*)|(`([^`\n]+?)`)|(\[([^\]\n]+)\]\(([^)\s]+)\))|((^|[^\w*])\*([^*\s][^*\n]*?)\*(?![\w*]))|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"])/g;
function Ye(e, t, n) {
	return _("a", {
		key: n,
		href: e,
		target: "_blank",
		rel: "noopener noreferrer nofollow"
	}, t);
}
function W(e) {
	let t = [], n = 0, r = 0, i = new RegExp(Je.source, "g");
	for (let a = i.exec(e); a; a = i.exec(e)) a.index > n && t.push(e.slice(n, a.index)), a[1] ? t.push(_("strong", { key: r++ }, W(a[2]))) : a[3] ? t.push(_("code", { key: r++ }, a[4])) : a[5] ? t.push(qe.test(a[7]) ? Ye(a[7], W(a[6]), r++) : a[6]) : a[8] ? (a[9] && t.push(a[9]), t.push(_("em", { key: r++ }, W(a[10])))) : a[11] && t.push(Ye(a[11], a[11], r++)), n = a.index + a[0].length;
	return n < e.length && t.push(e.slice(n)), t;
}
var Xe = /^\s{0,3}[-*+]\s+(.*)$/, Ze = /^\s{0,3}\d{1,3}[.)]\s+(.*)$/, Qe = /^\s{0,3}#{1,6}\s+(.*)$/, $e = /^\s{0,3}>\s?(.*)$/, et = /^\s{0,3}```/;
function tt(e, t = !1) {
	let n = Ge(e.replace(/\r\n?/g, "\n"), t);
	t && (n = Ke(n));
	let r = n.split("\n"), i = [], a = [], o = 0, s = () => {
		if (!a.length) return;
		let e = [];
		a.forEach((t, n) => {
			n && e.push(_("br", { key: `b${n}` })), e.push(...W(t));
		}), i.push(_("p", { key: o++ }, e)), a = [];
	};
	for (let e = 0; e < r.length; e++) {
		let t = r[e];
		if (et.test(t)) {
			s();
			let t = [];
			for (e++; e < r.length && !et.test(r[e]); e++) t.push(r[e]);
			i.push(_("pre", { key: o++ }, _("code", null, t.join("\n"))));
			continue;
		}
		let n = Xe.test(t) ? Xe : Ze.test(t) ? Ze : null;
		if (n) {
			s();
			let t = [];
			for (; e < r.length && n.test(r[e]); e++) t.push(_("li", { key: t.length }, W(n.exec(r[e])[1])));
			e--, i.push(_(n === Xe ? "ul" : "ol", { key: o++ }, t));
			continue;
		}
		let c = Qe.exec(t);
		if (c) s(), i.push(_("p", { key: o++ }, _("strong", null, W(c[1]))));
		else if ($e.test(t)) {
			s();
			let t = [];
			for (; e < r.length && $e.test(r[e]); e++) t.push($e.exec(r[e])[1]);
			e--, i.push(_("blockquote", { key: o++ }, W(t.join(" "))));
		} else t.trim() ? a.push(t.trim()) : s();
	}
	return s(), i;
}
function nt(e) {
	return Ge(e).replace(/```[\s\S]*?```/g, " ").replace(/\[([^\]]+)\]\([^)]+\)/g, "$1").replace(/[*_`#>]+/g, "").replace(/\s+/g, " ").trim();
}
//#endregion
//#region widget/src/realtime.ts
function rt(e, t) {
	try {
		let n = e.split(".")[1];
		if (!n) return null;
		let r = n.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(n.length / 4) * 4, "="), i = JSON.parse(atob(r))[t];
		return typeof i == "string" ? i : null;
	} catch {
		return null;
	}
}
var it = class {
	constructor(e, t, n, r, i, a) {
		this.factory = e, this.url = t, this.contactId = r, this.refresh = i, this.h = a, this.socket = null, this.channel = null, this.refreshing = !1, this.lastTyping = 0, this.token = n;
	}
	start() {
		let e = rt(this.token, "wid");
		if (!e || !this.url) return;
		let t = this.factory(this.url, () => ({ token: this.token }));
		this.socket = t;
		let n = !1;
		t.onOpen(() => {
			var e, t, r, i;
			(e = (t = this.h).onStatus) == null || e.call(t, !0), n && ((r = (i = this.h).onReconnect) == null || r.call(i)), n = !1;
		});
		let r = () => {
			var e, t;
			n = !0, (e = (t = this.h).onStatus) == null || e.call(t, !1), !this.refreshing && (this.refreshing = !0, this.refresh().then((e) => this.token = e).catch(() => {}).finally(() => this.refreshing = !1));
		};
		t.onError(r), t.onClose(r), t.connect();
		let i = t.channel(`contact:${e}:${this.contactId}`, {});
		i.on("event", (e) => this.h.onEvent(e)), i.on("typing", (e) => this.h.onTyping(e)), i.on("live", (e) => {
			var t, n;
			e && typeof e == "object" && typeof e.type == "string" && e.data && ((t = (n = this.h).onLive) == null || t.call(n, e));
		}), i.join(), this.channel = i;
	}
	typing(e) {
		let t = Date.now();
		!this.channel || t - this.lastTyping < 3e3 || (this.lastTyping = t, this.channel.push("typing", { ticket_id: e }));
	}
	typingDone() {
		this.lastTyping = 0;
	}
	stop() {
		var e, t;
		(e = this.channel) == null || e.leave(), (t = this.socket) == null || t.disconnect(), this.channel = null, this.socket = null;
	}
}, G = null;
function at() {
	var e;
	if (G) {
		G.state === "suspended" && G.resume().catch(() => {});
		return;
	}
	let t = (e = window.AudioContext) == null ? window.webkitAudioContext : e;
	if (t) try {
		G = new t();
	} catch {
		G = null;
	}
}
function ot() {
	if (!G || G.state !== "running") return;
	let e = G.currentTime, t = G.createGain();
	t.gain.value = .12, t.connect(G.destination), [[880, 0], [1318.5, .11]].forEach(([n, r]) => {
		let i = G.createOscillator(), a = G.createGain();
		i.type = "sine", i.frequency.value = n, a.gain.setValueAtTime(0, e + r), a.gain.linearRampToValueAtTime(.5, e + r + .012), a.gain.exponentialRampToValueAtTime(1e-4, e + r + .42), i.connect(a).connect(t), i.start(e + r), i.stop(e + r + .45);
	});
}
//#endregion
//#region widget/src/storage.ts
var st = {};
function ct(e) {
	var t;
	try {
		let t = window.localStorage.getItem(e);
		if (t !== null) return t;
	} catch {}
	return (t = st[e]) == null ? null : t;
}
function lt(e, t) {
	t === null ? delete st[e] : st[e] = t;
	try {
		t === null ? window.localStorage.removeItem(e) : window.localStorage.setItem(e, t);
	} catch {}
}
//#endregion
//#region node_modules/.pnpm/preact@11.0.1/node_modules/preact/hooks/dist/hooks.mjs
var ut, K, dt, ft, pt = Object.is, mt = 0, ht = [], gt = [], q = e, _t = q.__b, vt = q.__r, yt = q.diffed, bt = q.__c, xt = q.unmount, St = q.__;
function Ct(e, t) {
	q.__h && q.__h(K, e, mt || t), mt = 0;
	var n = K.__H || (K.__H = {
		__: [],
		__h: []
	});
	return e >= n.__.length && n.__.push({}), n.__[e];
}
function J(e) {
	return mt = 1, wt(Pt, e);
}
function wt(e, t, n) {
	var r = Ct(ut++, 2);
	if (r.t = e, !r.__c && (r.__ = [n ? n(t) : Pt(void 0, t), function(e) {
		var t = r.__N ? r.__N[0] : r.__[0], n = r.t(t, e);
		pt(t, n) || (r.__N = [n, r.__[1]], r.__c.setState({}));
	}], r.__c = K, !K.__f)) {
		K.__f = !0;
		var i = K.shouldComponentUpdate;
		K.shouldComponentUpdate = function(e, t, n) {
			var r = this.__H;
			if (!r) return !0;
			var a = !1, o = this.props != e;
			if (r.__.some(function(e) {
				e.__N && (a = !0, pt(e.__[0], e.__N[0]) || (o = !0));
			}), i) {
				var s = i.call(this, e, t, n);
				return a ? s || o : s;
			}
			return !a || o;
		};
	}
	return r.__;
}
function Y(e, t) {
	var n = Ct(ut++, 3);
	!q.__s && Nt(n.__H, t) && (n.__P = !0, n.__ = e, n.u = t, K.__H.__h.push(n));
}
function Tt(e, t) {
	var n = Ct(ut++, 4);
	!q.__s && Nt(n.__H, t) && (n.__P = !1, n.__ = e, n.u = t, K.__h.push(n));
}
function X(e) {
	return mt = 5, Et(function() {
		return { current: e };
	}, []);
}
function Et(e, t) {
	var n = Ct(ut++, 7);
	return Nt(n.__H, t) && (n.__ = e(), n.__H = t), n.__;
}
function Dt() {
	var e;
	do {
		for (; e = gt.shift();) try {
			jt(e);
		} catch (t) {
			q.__e(t, { __: (e = e.__P) && e.__v });
		}
		for (; e = ht.shift();) {
			var t = e.__H;
			if (e.__P && t) try {
				t.__h.some(jt), t.__h.some(Mt), t.__h = [];
			} catch (n) {
				t.__h = [], q.__e(n, e.__v);
			}
		}
	} while (gt.length);
}
q.__b = function(e) {
	K = null, _t && _t(e);
}, q.__ = function(e, t) {
	e && t.__k && t.__k.__m && (e.__m = t.__k.__m), St && St(e, t);
}, q.__r = function(e) {
	vt && vt(e), ut = 0;
	var t = (K = e.__c).__H;
	t && (dt == K ? K.__h = [] : (t.__h.some(jt), t.__h.some(Mt), ut = 0), t.__h = [], t.__.some(function(e) {
		e.__N && (e.__ = e.__N), e.u = e.__N = void 0;
	})), dt = K;
}, q.diffed = function(e) {
	yt && yt(e);
	var t = e.__c;
	t && t.__H && (t.__H.__h.length && At(ht.push(t)), t.__H.__.some(function(e) {
		e.u && (e.__H = e.u);
	})), dt = K = null;
}, q.__c = function(e, t) {
	t.some(function(e) {
		try {
			e.__h.some(jt), e.__h = e.__h.filter(function(e) {
				return !e.__ || Mt(e);
			});
		} catch (n) {
			t.some(function(e) {
				e.__h && (e.__h = []);
			}), t = [], q.__e(n, e.__v);
		}
	}), bt && bt(e, t);
}, q.unmount = function(e) {
	xt && xt(e);
	var t, n, r = e.__c;
	r && r.__H && (r.__H.__.some(function(r) {
		try {
			if (r.__P && r.__c) {
				if (n === void 0) {
					for (n = e.__; n && (!n.__c || !n.__c.__P);) n = n.__;
					n = n && n.__c;
				}
				r.__P = n, At(gt.push(r));
			} else jt(r);
		} catch (e) {
			t = e;
		}
	}), r.__H = void 0, t && q.__e(t, r.__v));
};
var Ot = typeof requestAnimationFrame == "function";
function kt(e) {
	var t, n = function() {
		clearTimeout(r), Ot && cancelAnimationFrame(t), setTimeout(e);
	}, r = setTimeout(n, 35);
	Ot && (t = requestAnimationFrame(n));
}
function At(e) {
	e != 1 && ft == q.requestAnimationFrame || ((ft = q.requestAnimationFrame) || kt)(Dt);
}
function jt(e) {
	var t = K, n = e.__c;
	typeof n == "function" && (e.__c = void 0, n()), K = t;
}
function Mt(e) {
	var t = K;
	e.__c = e.__(), K = t;
}
function Nt(e, t) {
	return !e || e.length != t.length || t.some(function(t, n) {
		return !pt(t, e[n]);
	});
}
function Pt(e, t) {
	return typeof t == "function" ? t(e) : t;
}
//#endregion
//#region widget/src/store.ts
var Ft = () => ({
	open: !1,
	view: "home",
	config: null,
	session: null,
	conversations: null,
	active: null,
	messages: {},
	typing: {},
	unread: 0,
	error: null,
	booting: !0,
	lang: "en",
	handoff: {},
	awaiting: {},
	live: {
		replies: {},
		closed: []
	},
	swapped: {},
	ratings: {},
	actions: {},
	preview: !1,
	dark: null
}), It = class {
	constructor(e = {}) {
		this.subs = /* @__PURE__ */ new Set(), this.state = {
			...Ft(),
			...e
		};
	}
	get() {
		return this.state;
	}
	set(e) {
		let t = typeof e == "function" ? e(this.state) : e;
		t && Object.keys(t).length !== 0 && (this.state = {
			...this.state,
			...t
		}, this.subs.forEach((e) => e()));
	}
	subscribe(e) {
		return this.subs.add(e), () => {
			this.subs.delete(e);
		};
	}
};
function Lt(e) {
	let [, t] = J(0);
	return Y(() => e.subscribe(() => t((e) => e + 1)), [e]), e.get();
}
//#endregion
//#region widget/src/controller.ts
var Rt = "__new", zt = 5e3, Bt = 3e4, Vt = 1e4, Ht = 200, Ut = 0, Wt = class {
	constructor(e) {
		var t;
		this.env = e, this.live = null, this.identity = {}, this.listeners = {
			message: /* @__PURE__ */ new Set(),
			open: /* @__PURE__ */ new Set(),
			close: /* @__PURE__ */ new Set()
		}, this.typingTimers = {}, this.sweepTimer = null, this.announce = () => {}, this.api = new Se(e.base, e.key, e.fetcher), this.store = (t = e.store) == null ? new It() : t, this.persist = e.persist !== !1, this.storageKey = `od_widget_${e.key}`, this.api.visitorToken = this.persist ? ct(this.storageKey) : null;
	}
	get s() {
		return this.store.get();
	}
	set(e) {
		this.store.set(e);
	}
	remember(e) {
		this.persist && lt(this.storageKey, e);
	}
	on(e, t) {
		var n;
		(n = this.listeners[e]) == null || n.add(t);
	}
	emit(e, t) {
		this.listeners[e].forEach((e) => {
			try {
				e(t);
			} catch {}
		});
	}
	async boot(e) {
		try {
			this.setConfig(e == null ? await this.api.config() : e), await this.startSession();
		} catch (e) {
			this.set({ error: e instanceof Error ? e.message : H("unavailable") });
		} finally {
			this.set({ booting: !1 });
		}
	}
	setConfig(e) {
		this.set({ config: he(e) });
	}
	async session() {
		try {
			return await this.api.session(this.identity);
		} catch (e) {
			if (e instanceof be && (e.status === 401 || e.status === 403 || e.status === 404) && this.api.visitorToken) return this.api.visitorToken = null, this.remember(null), this.api.session(this.identity);
			throw e;
		}
	}
	async startSession() {
		var e;
		let t = await this.session();
		this.remember(t.visitor_token), this.set({
			session: t,
			error: null
		}), (e = this.live) == null || e.stop(), this.live = new it(this.env.socketFactory, t.gateway_url, t.ws_token, t.contact.id, async () => {
			let e = await this.session();
			return this.set({ session: e }), e.ws_token;
		}, {
			onEvent: (e) => this.onEvent(e),
			onTyping: (e) => {
				var t, n;
				return this.onTyping(e.ticket_id, (t = (n = e.user) == null ? void 0 : n.name) == null ? H("support") : t);
			},
			onLive: (e) => this.onLive(e),
			onReconnect: () => void this.catchUp()
		}), this.live.start(), this.sweepTimer || (this.sweepTimer = setInterval(() => this.sweep(), 4e3)), await this.loadConversations();
	}
	async identify(e) {
		if (this.identity = {
			...this.identity,
			...e
		}, !this.s.booting) try {
			await this.startSession();
		} catch (e) {
			this.set({ error: e instanceof Error ? e.message : H("unavailable") });
		}
	}
	async catchUp() {
		await this.loadConversations();
		let e = this.s.active;
		e && await this.refresh(e);
	}
	async loadConversations() {
		try {
			let e = await this.api.conversations();
			e.sort((e, t) => Date.parse(t.last_message_at) - Date.parse(e.last_message_at)), this.set({ conversations: e });
		} catch {
			this.set((e) => {
				var t;
				return { conversations: (t = e.conversations) == null ? [] : t };
			});
		}
	}
	open() {
		this.s.open || (this.set({
			open: !0,
			unread: 0
		}), this.emit("open"));
	}
	close() {
		this.s.open && (this.set({ open: !1 }), this.emit("close"));
	}
	toggle() {
		this.s.open ? this.close() : this.open();
	}
	home() {
		this.set({
			view: "home",
			active: null
		}), this.loadConversations();
	}
	newConversation() {
		this.set((e) => {
			var t;
			return {
				view: "thread",
				active: null,
				messages: {
					...e.messages,
					[Rt]: (t = e.messages.__new) == null ? [] : t
				}
			};
		});
	}
	async startWith(e) {
		this.newConversation(), !this.needsEmail() && await this.send(e, []);
	}
	async openConversation(e) {
		this.set({
			view: "thread",
			active: e
		}), await this.refresh(e);
	}
	async refresh(e) {
		try {
			let t = await this.api.messages(e);
			this.applyMessages(e, t, !0);
		} catch {
			this.set((t) => {
				var n;
				return { messages: {
					...t.messages,
					[e]: (n = t.messages[e]) == null ? [] : n
				} };
			});
		}
	}
	applyMessages(e, t, n) {
		this.set((r) => {
			var i;
			let a = r.live, o = r.swapped;
			for (let e of t) {
				let t = Be(a, e);
				a = t.state, t.replaced && (o = {
					...o,
					[e.id]: !0
				});
			}
			let s = {
				live: a,
				swapped: o
			};
			(r.messages[e] || n) && (s.messages = {
				...r.messages,
				[e]: Kt((i = r.messages[e]) == null ? [] : i, t)
			});
			let c = r.ratings, l = r.actions;
			for (let e of t) {
				var u;
				e.id in c && ((u = e.ai_rating) == null ? null : u) === c[e.id] && (c = { ...c }, delete c[e.id]);
				let t = e.ai_action;
				t && t.status !== "pending" && l[t.id] && (l = { ...l }, delete l[t.id]);
			}
			return s.ratings = c, s.actions = l, s;
		});
	}
	async handoff(e) {
		if (this.s.handoff[e] !== "pending" && this.s.handoff[e] !== "done") {
			this.set((t) => ({ handoff: {
				...t.handoff,
				[e]: "pending"
			} }));
			try {
				await this.api.handoff(e), this.markHandedOff(e), this.announce(H("handoffDone"));
			} catch {
				this.set((t) => ({ handoff: {
					...t.handoff,
					[e]: "failed"
				} }));
			}
		}
	}
	markHandedOff(e) {
		this.set((t) => {
			var n;
			let r = { ...t.awaiting };
			return delete r[e], {
				handoff: {
					...t.handoff,
					[e]: "done"
				},
				awaiting: r,
				conversations: ((n = t.conversations) == null ? [] : n).map((t) => t.id === e ? {
					...t,
					ai_handoff: !0
				} : t)
			};
		});
	}
	async decide(e, t, n) {
		let r = this.s.actions[t.id];
		if (!(r != null && r.busy || r != null && r.status)) {
			this.set((e) => ({ actions: {
				...e.actions,
				[t.id]: { busy: n }
			} }));
			try {
				await this.api.decide(e, t.id, n), this.set((r) => ({
					actions: {
						...r.actions,
						[t.id]: { status: n === "confirm" ? "confirmed" : "declined" }
					},
					awaiting: {
						...r.awaiting,
						[e]: Date.now()
					}
				}));
			} catch (n) {
				n instanceof be && n.status === 409 ? (this.set((e) => {
					let n = { ...e.actions };
					return delete n[t.id], { actions: n };
				}), this.refresh(e)) : this.set((e) => ({ actions: {
					...e.actions,
					[t.id]: { error: !0 }
				} }));
			}
		}
	}
	async rate(e, t, n) {
		var r;
		let i = (t.id in this.s.ratings ? this.s.ratings[t.id] : (r = t.ai_rating) == null ? null : r) === n ? null : n;
		this.set((e) => ({ ratings: {
			...e.ratings,
			[t.id]: i
		} }));
		try {
			await this.api.rate(e, t.id, i);
		} catch {
			this.set((e) => {
				if (e.ratings[t.id] !== i) return {};
				let n = { ...e.ratings };
				return delete n[t.id], { ratings: n };
			});
		}
	}
	async captureEmail(e, t) {
		this.identity = {
			...this.identity,
			name: e || void 0,
			email: t
		};
		let n = await this.session();
		this.remember(n.visitor_token), this.set({ session: n });
	}
	needsEmail() {
		let { config: e, session: t } = this.s;
		return !!(e != null && e.require_email) && !!t && !t.contact.email;
	}
	typing() {
		var e;
		let { active: t } = this.s;
		t && ((e = this.live) == null || e.typing(t));
	}
	async send(e, t) {
		var n;
		let r = e.trim();
		if (!r && !t.length) return;
		let { active: i, session: a } = this.s, o = i == null ? Rt : i, s = {
			id: `local-${++Ut}`,
			ticket_id: o,
			kind: "reply",
			direction: "inbound",
			author: {
				type: "contact",
				user: null,
				contact: (n = a == null ? void 0 : a.contact) == null ? null : n
			},
			body_html: "",
			body_text: r,
			attachments: t.map((e, t) => ({
				id: `local-file-${Ut}-${t}`,
				filename: e.name,
				content_type: e.type,
				size_bytes: e.size,
				inline: !1,
				url: e.type.startsWith("image/") ? URL.createObjectURL(e) : ""
			})),
			created_at: (/* @__PURE__ */ new Date()).toISOString(),
			local: "pending",
			localFiles: t
		};
		this.set((e) => {
			var t;
			return { messages: {
				...e.messages,
				[o]: [...(t = e.messages[o]) == null ? [] : t, s]
			} };
		}), await this.deliver(o, s);
	}
	async retry(e, t) {
		var n;
		let r = (n = this.s.messages[e]) == null ? void 0 : n.find((e) => e.id === t);
		r && (this.patchLocal(e, t, { local: "pending" }), await this.deliver(e, r));
	}
	patchLocal(e, t, n) {
		this.set((r) => {
			var i;
			return { messages: {
				...r.messages,
				[e]: ((i = r.messages[e]) == null ? [] : i).map((e) => e.id === t ? {
					...e,
					...n
				} : e)
			} };
		});
	}
	async deliver(e, t) {
		var n;
		(n = this.live) == null || n.typingDone();
		let r = this.s.preview ? void 0 : ye();
		try {
			var i;
			let n = [];
			for (let e of (i = t.localFiles) == null ? [] : i) n.push(await this.api.upload(e));
			let a = n.map((e) => e.id);
			if (e === "__new") {
				let { conversation: e, message: n } = await this.api.start(t.body_text, a.length ? a : void 0, r);
				this.set((t) => {
					var r, i;
					let a = { ...t.messages };
					delete a[Rt];
					let o = n ? [n] : [];
					return {
						active: t.active === null && t.view === "thread" ? e.id : t.active,
						messages: {
							...a,
							[e.id]: Kt((r = t.messages[e.id]) == null ? [] : r, o)
						},
						conversations: [e, ...((i = t.conversations) == null ? [] : i).filter((t) => t.id !== e.id)],
						awaiting: {
							...t.awaiting,
							[e.id]: Date.now()
						}
					};
				}), n || this.openConversation(e.id);
			} else {
				let n = await this.api.send(e, t.body_text, a.length ? a : void 0, r);
				this.set((r) => {
					var i;
					return {
						messages: {
							...r.messages,
							[e]: Gt(((i = r.messages[e]) == null ? [] : i).filter((e) => e.id !== t.id), n)
						},
						awaiting: {
							...r.awaiting,
							[e]: Date.now()
						}
					};
				}), this.touchConversation(e, n);
			}
		} catch {
			this.patchLocal(e, t.id, { local: "failed" });
		}
	}
	touchConversation(e, t) {
		this.set((n) => {
			var r;
			let i = (r = n.conversations) == null ? [] : r, a = i.find((t) => t.id === e);
			return a ? { conversations: [{
				...a,
				last_message_at: t.created_at,
				last_message_preview: t.body_text.slice(0, 140)
			}, ...i.filter((t) => t.id !== e)] } : {};
		});
	}
	onTyping(e, t) {
		this.set((n) => ({ typing: {
			...n.typing,
			[e]: t
		} })), clearTimeout(this.typingTimers[e]), this.typingTimers[e] = setTimeout(() => this.clearTyping(e), zt);
	}
	clearTyping(e) {
		clearTimeout(this.typingTimers[e]), this.set((t) => {
			if (!(e in t.typing)) return {};
			let n = { ...t.typing };
			return delete n[e], { typing: n };
		});
	}
	clearAwaiting(e) {
		this.set((t) => {
			if (!(e in t.awaiting)) return {};
			let n = { ...t.awaiting };
			return delete n[e], { awaiting: n };
		});
	}
	onLive(e) {
		var t;
		let n = this.s.live, r = (t = e.data) == null ? void 0 : t.ticket_id;
		if (this.set((t) => ({ live: ze(t.live, e, Date.now(), (e) => Object.values(t.messages).some((t) => t.some((t) => t.id === e))) })), e.type !== "ai.reply.ended" || this.s.live === n || typeof r != "string") return;
		this.clearAwaiting(r), e.data.handoff && this.markHandedOff(r);
		let i = this.s.live.replies[e.data.reply_id];
		(i == null ? void 0 : i.phase) === "leaving" && setTimeout(() => this.set((e) => ({ live: Ve(e.live, { id: i.id }) })), Ht);
	}
	sweep() {
		let e = Date.now(), { live: t, active: n } = this.s, r = Object.values(t.replies).filter((t) => t.phase === "done" ? t.at < e - Vt : t.at < e - Bt);
		r.length && (this.set((t) => ({ live: Ve(t.live, {
			idleBefore: e - Bt,
			doneBefore: e - Vt
		}) })), n && r.some((e) => e.ticket_id === n) && this.refresh(n));
	}
	onEvent(e) {
		var t, n, r, i, a, o, s;
		if (e.type !== "message.created" && e.type !== "message.updated") return;
		let c = e.data.message, l = (t = e.data.ticket_id) == null ? c == null ? void 0 : c.ticket_id : t;
		if (!c || !l || c.kind === "note") return;
		let u = this.s;
		if (this.applyMessages(l, [c], !1), e.type === "message.updated" || (u.conversations && !u.conversations.some((e) => e.id === l) ? this.loadConversations() : this.touchConversation(l, c), c.direction !== "outbound")) return;
		this.clearTyping(l), this.clearAwaiting(l);
		let d = u.open && u.view === "thread" && u.active === l;
		u.open ? d || this.loadConversations() : this.set((e) => ({ unread: e.unread + 1 })), (n = u.config) != null && n.sound && !u.preview && typeof document < "u" && document.hidden && ot();
		let f = Ce(c), p = (r = (i = c.author.user) == null ? void 0 : i.name) == null ? f ? (a = (o = u.config) == null || (o = o.bot) == null ? void 0 : o.name) == null ? H("ai") : a : ((s = u.config) == null ? void 0 : s.workspace_name) || H("support") : r, m = (c.body_text || nt(c.body_html.replace(/<[^>]+>/g, " "))).slice(0, 200);
		this.announce(H("newMessage", {
			name: p,
			text: m
		})), this.emit("message", {
			conversation_id: l,
			body: c.body_text,
			from: p
		});
	}
	shutdown() {
		this.stop(), this.remember(null), this.api.visitorToken = null;
	}
	stop() {
		var e;
		(e = this.live) == null || e.stop(), this.live = null, this.sweepTimer && clearInterval(this.sweepTimer), this.sweepTimer = null, Object.values(this.typingTimers).forEach(clearTimeout);
	}
};
function Gt(e, t) {
	let n = e.findIndex((e) => e.id === t.id);
	return n >= 0 ? e.map((e, r) => r === n ? t : e) : [...e, t].sort((e, t) => Date.parse(e.created_at) - Date.parse(t.created_at));
}
function Kt(e, t) {
	let n = e;
	for (let e of t) {
		if (e.direction === "inbound") {
			let t = n.find((t) => t.local === "pending" && t.body_text === e.body_text);
			t && !n.some((t) => t.id === e.id) && (n = n.filter((e) => e !== t));
		}
		n = Gt(n, e);
	}
	return n;
}
//#endregion
//#region widget/src/mock.ts
var qt = "00000000-0000-4000-8000-00000000d3e0", Jt = "https://example.com/help/margin/articles", Yt = {
	en: {
		subject: "Exporting notebooks to PDF",
		q1: "Hi! Is there a way to export a whole notebook to PDF, not just single pages?",
		a1: "<p>Yes. Open the notebook, then choose <strong>File → Export → PDF</strong> and pick <em>Entire notebook</em>.</p>",
		q2: "Perfect, that did it. Thanks!",
		a2: "Glad that worked! Let us know if anything else comes up.",
		aiSso: "Sign-in loops with SSO usually mean the identity provider's **signing certificate** changed [1]. An admin can upload the new certificate under **Settings → Security → SSO** [2], then everyone can sign in again.\n\nWhile that's sorted, admins can still sign in with email and password.",
		s1: "Set up SAML single sign-on",
		s2: "Rotate your SSO certificate",
		human: "Hi, Priya here from the Margin team. I've picked this up and I'm on it now."
	},
	ja: {
		subject: "ノートをPDFで書き出す",
		q1: "こんにちは。ページ単位ではなく、ノート全体をPDFで書き出す方法はありますか？",
		a1: "<p>はい。ノートを開いて <strong>ファイル → 書き出し → PDF</strong> を選び、<em>ノート全体</em> を指定してください。</p>",
		q2: "できました。ありがとうございます！",
		a2: "うまくいってよかったです。ほかにも何かあればお気軽にどうぞ。",
		aiSso: "SSOでサインインが繰り返される場合、IDプロバイダーの**署名証明書**が更新された可能性があります [1]。管理者が **設定 → セキュリティ → SSO** から新しい証明書をアップロードすると [2]、再びサインインできるようになります。",
		s1: "SAMLシングルサインオンを設定する",
		s2: "SSO証明書を更新する",
		human: "Marginサポートのプリヤです。こちらで引き継ぎました。"
	},
	ar: {
		subject: "تصدير الدفاتر بصيغة PDF",
		q1: "مرحبًا! هل يمكن تصدير دفتر كامل بصيغة PDF، وليس صفحات منفردة فقط؟",
		a1: "<p>نعم. افتح الدفتر، ثم اختر <strong>ملف ← تصدير ← PDF</strong> وحدّد <em>الدفتر بالكامل</em>.</p>",
		q2: "ممتاز، نجح الأمر. شكرًا!",
		a2: "يسعدنا ذلك! أخبرنا إن احتجت إلى أي شيء آخر.",
		aiSso: "تكرار تسجيل الدخول عبر SSO يعني غالبًا أن **شهادة التوقيع** لدى مزوّد الهوية قد تغيّرت [1]. يمكن للمسؤول رفع الشهادة الجديدة من **الإعدادات ← الأمان ← SSO** [2]، وبعدها يستطيع الجميع تسجيل الدخول مجددًا.",
		s1: "إعداد تسجيل الدخول الموحّد SAML",
		s2: "تحديث شهادة SSO",
		human: "مرحبًا، معك بريا من فريق Margin. تسلّمت محادثتك وأعمل عليها الآن."
	}
}, Z = {
	question: "Can you send order #4821 to my new address? It's 22 Baker Street, London.",
	lookup: "I found order **#4821**: a Margin Pro notebook set, not shipped yet, so the address can still change.\n\nShall I send it to **22 Baker Street, London** instead?",
	action: {
		title: "Change the delivery address",
		summary: "Send order #4821 to 22 Baker Street, London instead of 9 Elm Road.",
		tool: "Orders"
	},
	confirmed: "Done. Order **#4821** will now ship to 22 Baker Street, London. You'll get an updated confirmation email in a minute.",
	declined: "No problem, I've left order #4821 as it was. Anything else I can help with?"
};
function Xt(e, t = new URLSearchParams()) {
	var n, r;
	let i = {
		workspace_name: "Margin",
		brand_color: null,
		greeting: "Hi there. How can we help?",
		help_center_url: "https://example.com/help/margin",
		require_email: !1,
		locale: (n = t.get("od_ws_lang")) == null ? "en" : n,
		avatar_url: null,
		launcher_icon_url: null,
		theme: "auto",
		position: "right",
		offset_x: 20,
		offset_y: 20,
		launcher: "icon",
		launcher_label: "",
		radius: "soft",
		title: "",
		subtitle: "",
		suggestions: [
			"Where is my order?",
			"Change my delivery address",
			"I can't sign in with SSO",
			"Talk to a person"
		],
		sound: !0,
		team: [
			{
				name: "Priya",
				avatar_url: null
			},
			{
				name: "Tomás",
				avatar_url: null
			},
			{
				name: "Mei",
				avatar_url: null
			}
		],
		ai_answers: t.get("od_bot") !== "0",
		bot: {
			name: "Ollie",
			avatar_url: null
		}
	}, a = {
		wk_brand: { brand_color: "#3b5bdb" },
		wk_email: { require_email: !0 },
		wk_left: {
			position: "left",
			offset_x: 32,
			offset_y: 28,
			brand_color: "#0f766e"
		},
		wk_label: {
			launcher: "label",
			launcher_label: "",
			brand_color: "#7c3aed"
		},
		wk_round: {
			radius: "round",
			launcher: "label",
			launcher_label: "Ask us",
			brand_color: "#e8590c"
		},
		wk_square: {
			radius: "square",
			brand_color: "#111827"
		},
		wk_dark: {
			theme: "dark",
			brand_color: "#a3e635",
			title: "Margin support",
			subtitle: "Ask Ollie anything, day or night."
		},
		wk_light: { theme: "light" },
		wk_human: {
			ai_answers: !1,
			suggestions: []
		}
	};
	return {
		...i,
		...(r = a[e]) == null ? {} : r
	};
}
function Zt(e = {}) {
	var t, n, r;
	let i = (t = Yt[(n = e.lang) == null ? "" : n]) == null ? Yt.en : t, a = {
		id: "00000000-0000-4000-8000-0000000c0de0",
		email: null,
		name: "",
		avatar_url: ""
	}, o = {
		id: "u_priya",
		name: "Priya Raman",
		avatar_url: ""
	}, s = (r = e.latency) == null ? 120 : r, c = /* @__PURE__ */ new Set(), l = (e, t) => {
		let n = setTimeout(() => {
			c.delete(n), t();
		}, e);
		c.add(n);
	}, u = (e) => (/* @__PURE__ */ new Date(Date.now() - e * 6e4)).toISOString(), d = (e) => btoa(JSON.stringify(e)).replace(/=+$/, "").replace(/\+/g, "-").replace(/\//g, "_"), f = () => `${d({
		alg: "HS256",
		typ: "JWT"
	})}.${d({
		purpose: "widget_ws",
		sub: a.id,
		wid: qt,
		exp: Date.now() / 1e3 + 300
	})}.mock`, p = 0, m = () => `${((++p).toString(16).padStart(4, "0") + Math.random().toString(16).slice(2, 10)).slice(0, 8)}-0000-4000-8000-${Date.now().toString(16).slice(-12).padStart(12, "0")}`;
	function h(e, t, n, r = 0, i = "") {
		return {
			id: m(),
			ticket_id: e,
			kind: "reply",
			direction: t,
			author: t === "inbound" ? {
				type: "contact",
				user: null,
				contact: a
			} : {
				type: "user",
				user: o,
				contact: null
			},
			body_html: i,
			body_text: n,
			attachments: [],
			created_at: u(r),
			ai: !1,
			ai_action: null,
			ai_rating: null
		};
	}
	function g(e, t, n = [], r = null, i = 0) {
		let a = t.replace(/\s*\[\d+\]/g, ""), o = (e) => e.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;"), s = a.split(/\n{2,}/).map((e) => `<p>${o(e).replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")}</p>`).join("") + (n.length ? `<ul>${n.map((e) => `<li><a href="${e.url}">${o(e.title)}</a></li>`).join("")}</ul>` : "");
		return {
			...h(e, "outbound", a.replace(/\*\*/g, ""), i, s),
			author: {
				type: "system",
				user: null,
				contact: null
			},
			ai: !0,
			ai_action: r ? {
				...r,
				id: m(),
				status: "pending"
			} : null
		};
	}
	let _ = [], v = {}, y = m();
	if (e.seed === "preview") {
		let e = h(y, "inbound", "Hi! Can I still change where my order ships?", 3), t = g(y, "Yes, as long as it hasn't shipped yet [1]. Tell me the order number and the new address, and I'll take care of it.", [{
			title: "Changing a delivery address",
			url: `${Jt}/change-address`
		}], null, 2.8), n = h(y, "inbound", Z.question, 1);
		_.push({
			id: y,
			subject: "Changing my delivery address",
			status: "open",
			last_message_preview: Z.question,
			last_message_at: n.created_at
		}), v[y] = [
			e,
			t,
			n
		];
	} else if (e.seed !== "empty") {
		let e = m();
		_.push({
			id: e,
			subject: i.subject,
			status: "solved",
			last_message_preview: i.a2,
			last_message_at: u(1560)
		}), v[e] = [
			h(e, "inbound", i.q1, 1620),
			h(e, "outbound", "", 1590, i.a1),
			h(e, "inbound", i.q2, 1572),
			h(e, "outbound", i.a2, 1560)
		];
	}
	let b = [], x = (e, t) => {
		var n;
		for (let r of b) ((n = r.handlers[e]) == null ? [] : n).forEach((e) => e(t));
	}, S = (e, t) => x("event", {
		id: m(),
		seq: p,
		type: e,
		data: {
			ticket_id: t.ticket_id,
			message: t
		},
		topics: []
	}), C = (e, t) => x("live", {
		type: e,
		data: t
	}), w = (e) => _.find((t) => t.id === e);
	function T(e) {
		var t, n;
		((n = v[t = e.ticket_id]) == null ? v[t] = [] : n).push(e);
		let r = w(e.ticket_id);
		r && (r.last_message_at = e.created_at, r.last_message_preview = e.body_text.slice(0, 140)), S("message.created", e);
	}
	function E(e, t, n = {}) {
		var r, i, a, o;
		let s = m(), c = (r = n.delay) == null ? 700 : r;
		l(c, () => C("ai.reply.started", {
			reply_id: s,
			ticket_id: e,
			bot: { name: "Ollie" }
		})), n.tool && (c += 500, l(c, () => C("ai.reply.tool", {
			reply_id: s,
			ticket_id: e,
			tool: n.tool,
			status: "running"
		})), c += 1300, l(c, () => C("ai.reply.tool", {
			reply_id: s,
			ticket_id: e,
			tool: n.tool,
			status: "done"
		}))), c += 400;
		let u = (i = t.match(/\S+\s*/g)) == null ? [] : i;
		for (let t = 0; t < u.length;) {
			let n = 1 + t * 7 % 3, r = u.slice(t, t + n).join("");
			t += n, c += 45 + t * 13 % 60, l(c, () => C("ai.reply.delta", {
				reply_id: s,
				ticket_id: e,
				text: r
			}));
		}
		c += 250;
		let d = g(e, t, (a = n.sources) == null ? [] : a, (o = n.action) == null ? null : o);
		l(c, () => {
			var t, r;
			if (C("ai.reply.ended", {
				reply_id: s,
				ticket_id: e,
				message_id: d.id,
				handoff: !!n.handoff,
				action_id: (t = (r = d.ai_action) == null ? void 0 : r.id) == null ? null : t
			}), n.handoff) {
				let t = w(e);
				t && (t.ai_handoff = !0);
			}
		}), l(c + 120, () => T(d)), n.handoff && D(e, c + 3500);
	}
	function D(e, t = 1500) {
		l(t, () => x("typing", {
			ticket_id: e,
			user: {
				id: o.id,
				name: "Priya"
			}
		})), l(t + 2600, () => T(h(e, "outbound", i.human)));
	}
	function O(e, t) {
		var n;
		let r = w(e);
		!r || r.ai_handoff || (n = v[e]) != null && n.some((e) => e.author.type === "user") || (/person|human|someone|agent|real/i.test(t) ? E(e, "Of course. I've asked the team to join; a person will reply right here.", { handoff: !0 }) : /order|address|deliver|ship|#\d+/i.test(t) ? /#?\d{4}|street|road|avenue/i.test(t) ? E(e, Z.lookup, {
			tool: "Order lookup",
			action: Z.action,
			delay: 900
		}) : E(e, "Happy to help with that. What's your order number, and where should it go?", { tool: "Order lookup" }) : /sso|sign|log ?in|saml|ログイン|サインイン|تسجيل/i.test(t) ? E(e, i.aiSso, {
			tool: "Knowledge search",
			sources: [{
				title: i.s1,
				url: `${Jt}/saml-sso`
			}, {
				title: i.s2,
				url: `${Jt}/rotate-sso-certificate`
			}]
		}) : E(e, "Thanks for the message! I can look up **orders**, change a delivery address, or point you to the right guide [1]. What do you need?", { sources: [{
			title: "Getting started with Margin",
			url: `${Jt}/getting-started`
		}] }));
	}
	let k = typeof location > "u" ? new URLSearchParams() : new URLSearchParams(location.search), A = () => k.get("od_bot") !== "0", j = [
		"Thanks for reaching out! Let me take a look at that for you.",
		"Good question. You can find that under Settings → Workspace. Want me to walk you through it?",
		"I've passed this on to our engineers and will update you here as soon as I hear back."
	], M = 0;
	function ee(e) {
		l(900, () => x("typing", {
			ticket_id: e,
			user: {
				id: o.id,
				name: "Priya"
			}
		})), l(3200, () => x("typing", {
			ticket_id: e,
			user: {
				id: o.id,
				name: "Priya"
			}
		})), l(4600, () => T(h(e, "outbound", j[M++ % j.length])));
	}
	let N = (e, t = 200) => new Response(t === 204 ? null : JSON.stringify(e), {
		status: t,
		headers: { "content-type": "application/json" }
	}), te = () => N({
		status: 404,
		title: "Not found",
		code: "not_found"
	}, 404), P = async (t, n) => {
		var r, i, o;
		await new Promise((e) => setTimeout(e, s));
		let c = new URL(t, typeof location > "u" ? "https://mock.invalid" : location.href), d = c.pathname, p = ((r = n.method) == null ? "GET" : r).toUpperCase(), g = typeof n.body == "string" ? JSON.parse(n.body) : {};
		if (d === "/v1/widget/config") return N(((i = e.config) == null ? ((e) => Xt(e, k)) : i)((o = c.searchParams.get("key")) == null ? "" : o));
		if (d === "/v1/widget/session") return typeof g.email == "string" && (a.email = g.email), typeof g.name == "string" && (a.name = g.name), N({
			visitor_token: "od_vis_mock",
			contact: { ...a },
			ws_token: f(),
			gateway_url: "wss://mock.invalid/socket/widget"
		});
		if (d === "/v1/widget/conversations" && p === "GET") return N({
			data: _,
			next_cursor: null
		});
		if (d === "/v1/widget/conversations" && p === "POST") {
			var y;
			let e = m(), t = String((y = g.body) == null ? "" : y), n = {
				id: e,
				subject: t.slice(0, 60) || "New conversation",
				status: "open",
				last_message_preview: t,
				last_message_at: u(0)
			};
			_.unshift(n);
			let r = h(e, "inbound", t);
			return v[e] = [r], A() ? O(e, t) : ee(e), N({
				conversation: n,
				message: r
			}, 201);
		}
		let b = /^\/v1\/widget\/conversations\/([^/]+)\/handoff$/.exec(d);
		if (b && p === "POST") {
			let e = w(b[1]);
			return e ? (e.ai_handoff || (e.ai_handoff = !0, D(e.id)), N(null, 204)) : te();
		}
		if (b = /^\/v1\/widget\/conversations\/([^/]+)\/actions\/([^/]+)$/.exec(d), b && p === "POST") {
			var x;
			let [, e, t] = b, n = (x = v[e]) == null ? void 0 : x.find((e) => {
				var n;
				return ((n = e.ai_action) == null ? void 0 : n.id) === t;
			});
			if (!(n != null && n.ai_action)) return te();
			if (n.ai_action.status !== "pending") return N({
				status: 409,
				title: "Conflict",
				code: "action_not_pending"
			}, 409);
			let r = g.decision === "confirm";
			return n.ai_action = {
				...n.ai_action,
				status: r ? "confirmed" : "declined"
			}, l(300, () => S("message.updated", n)), E(e, r ? Z.confirmed : Z.declined, r ? {
				tool: "Orders",
				delay: 500
			} : { delay: 500 }), N(null, 204);
		}
		if (b = /^\/v1\/widget\/conversations\/([^/]+)\/messages\/([^/]+)\/rating$/.exec(d), b && p === "POST") {
			var C;
			let e = (C = v[b[1]]) == null ? void 0 : C.find((e) => e.id === b[2] && e.ai);
			return e ? (e.ai_rating = g.rating === "up" || g.rating === "down" ? g.rating : null, l(200, () => S("message.updated", e)), N(null, 204)) : te();
		}
		if (b = /^\/v1\/widget\/conversations\/([^/]+)\/messages$/.exec(d), b) {
			var T, j, M, P, F;
			let e = b[1];
			if (p === "GET") return N({
				data: (T = v[e]) == null ? [] : T,
				next_cursor: null
			});
			let t = String((j = g.body) == null ? "" : j), n = h(e, "inbound", t);
			for (let t of (M = v[e]) == null ? [] : M) ((P = t.ai_action) == null ? void 0 : P.status) === "pending" && (t.ai_action = {
				...t.ai_action,
				status: "expired"
			}, l(150, () => S("message.updated", t)));
			((F = v[e]) == null ? v[e] = [] : F).push(n);
			let r = w(e);
			return r && (r.last_message_at = n.created_at, r.last_message_preview = t), r && !r.ai_handoff && A() && !v[e].some((e) => e.author.type === "user") ? O(e, t) : ee(e), N(n, 201);
		}
		if (d === "/v1/widget/attachments" && n.body instanceof FormData) {
			let e = n.body.get("file");
			return N({
				id: m(),
				filename: e.name,
				content_type: e.type,
				size_bytes: e.size,
				inline: !1,
				url: URL.createObjectURL(e)
			}, 201);
		}
		return te();
	}, F = () => {
		let e = [];
		return {
			connect: () => l(50, () => e.forEach((e) => e())),
			disconnect: () => {
				b.length = 0;
			},
			onOpen: (t) => e.push(t),
			onError: () => {},
			onClose: () => {},
			channel(e) {
				let t = {
					topic: e,
					handlers: {}
				};
				b.push(t);
				let n = { receive: (e, t) => (e === "ok" && l(30, () => t({})), n) };
				return {
					join: () => n,
					on: (e, n) => {
						var r, i;
						return ((i = (r = t.handlers)[e]) == null ? r[e] = [] : i).push(n), 0;
					},
					push: () => n,
					leave: () => {
						let e = b.indexOf(t);
						e >= 0 && b.splice(e, 1);
					}
				};
			}
		};
	}, ne = !1;
	return {
		fetch: P,
		socket: F,
		showcaseId: y,
		playShowcase() {
			ne || (ne = !0, E(y, Z.lookup, {
				tool: "Order lookup",
				action: Z.action,
				delay: 500
			}));
		},
		dispose() {
			c.forEach(clearTimeout), c.clear(), b.length = 0;
		}
	};
}
//#endregion
//#region widget/src/styles.ts
var Qt = "\n  --bg: #ffffff; --bg-2: #f4f4f5; --bg-3: #fafafa; --line: #e7e7e9; --line-2: #d6d6d9;\n  --fg: #0a0a0a; --fg-2: #525252; --fg-3: #737373; --fg-4: #a3a3a3;\n  --accent: var(--brand, #0a0a0a); --accent-fg: var(--brand-fg, #ffffff);\n  --ai: #6d4bd8; --ai-soft: #f4f0ff; --ai-line: #e4dcff;\n  --ok: #15803d; --ok-soft: #ecfdf3; --danger: #dc2626; --danger-soft: #fef2f2;\n  --shadow: 0 0 0 1px rgb(10 10 10 / .06), 0 12px 28px -8px rgb(10 10 10 / .18), 0 32px 64px -20px rgb(10 10 10 / .24);\n  --l-shadow: 0 0 0 1px rgb(255 255 255 / .08) inset, 0 8px 20px -6px rgb(0 0 0 / .3), 0 2px 4px rgb(0 0 0 / .08);\n  --ring: color-mix(in srgb, var(--fg) 8%, transparent);\n  color-scheme: light;\n", $t = "\n  --bg: #161616; --bg-2: #242426; --bg-3: #1c1c1d; --line: #2c2c2f; --line-2: #3a3a3e;\n  --fg: #fafafa; --fg-2: #c4c4c6; --fg-3: #8d8d92; --fg-4: #5e5e63;\n  --accent: var(--brand, #fafafa); --accent-fg: var(--brand-fg, #0a0a0a);\n  --ai: #b9a3ff; --ai-soft: #231d33; --ai-line: #372d52;\n  --ok: #4ade80; --ok-soft: #12261a; --danger: #f87171; --danger-soft: #2a1515;\n  --shadow: 0 0 0 1px rgb(255 255 255 / .08), 0 12px 28px -8px rgb(0 0 0 / .6), 0 32px 64px -20px rgb(0 0 0 / .7);\n  --l-shadow: 0 0 0 1px rgb(255 255 255 / .14), 0 8px 20px -6px rgb(0 0 0 / .6);\n  --ring: color-mix(in srgb, var(--fg) 12%, transparent);\n  color-scheme: dark;\n", en = `
:host { all: initial; }
.root {
  ${Qt}
  --ease: cubic-bezier(.16, 1, .3, 1);
  --font: system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Hiragino Sans", "Hiragino Kaku Gothic ProN", "Yu Gothic UI", Meiryo, "PingFang SC", "Microsoft YaHei", "Apple SD Gothic Neo", "Malgun Gothic", "Noto Sans CJK SC", "Noto Sans CJK JP", "Noto Sans CJK KR", "Segoe UI Arabic", "Geeza Pro", "Noto Sans Arabic", sans-serif;
  --mono: ui-monospace, "SF Mono", Menlo, Consolas, monospace;
  --gap-launcher: calc(var(--oy, 20px) + 66px);
  font-family: var(--font); font-size: 14px; line-height: 20px; color: var(--fg); -webkit-font-smoothing: antialiased;
}
.root[data-theme="dark"] { ${$t} }
@media (prefers-color-scheme: dark) { .root[data-theme="auto"] { ${$t} } }
* { box-sizing: border-box; margin: 0; padding: 0; }
button, textarea, input { font: inherit; color: inherit; }
button { cursor: pointer; background: none; border: 0; }
:focus { outline: none; }
:focus-visible { outline: 2px solid color-mix(in srgb, var(--fg) 30%, transparent); outline-offset: 2px; }
textarea:focus-visible, input:focus-visible { outline: none; }
img { display: block; }

/* Launcher: icon, or a pill with the icon and a label. */
.launcher {
  position: fixed; right: var(--ox, 20px); bottom: var(--oy, 20px); z-index: 1;
  height: 54px; min-width: 54px; padding: 0 15px; border-radius: var(--r-launcher, 16px);
  background: var(--accent); color: var(--accent-fg); display: flex; align-items: center; justify-content: center; gap: 0;
  box-shadow: var(--l-shadow); pointer-events: auto;
  transition: transform .2s var(--ease), box-shadow .2s, padding .22s var(--ease), gap .22s var(--ease);
  animation: launcher-in .24s var(--ease) both;
}
.pos-left .launcher { right: auto; left: var(--ox, 20px); }
.launcher:hover { transform: translateY(-1px) scale(1.03); }
.launcher:active { transform: scale(.95); transition-duration: .08s; }
.launcher.pill { height: 48px; padding: 0 18px 0 15px; gap: 8px; }
.launcher.pill[aria-expanded="true"] { padding: 0 14px; gap: 0; }
.l-ic { position: relative; width: 24px; height: 24px; flex: none; }
.pill .l-ic { width: 20px; height: 20px; }
.l-ic .ic { position: absolute; inset: 0; display: grid; place-items: center; transition: transform .22s var(--ease), opacity .15s; }
.l-ic img { width: 100%; height: 100%; object-fit: cover; border-radius: 5px; }
.launcher .ic-x { opacity: 0; transform: rotate(-60deg) scale(.6); }
.launcher[aria-expanded="true"] .ic-open { opacity: 0; transform: rotate(60deg) scale(.6); }
.launcher[aria-expanded="true"] .ic-x { opacity: 1; transform: none; }
.l-label { display: block; max-width: 220px; overflow: hidden; white-space: nowrap; font-size: 14px; font-weight: 600; letter-spacing: -.01em; transition: max-width .22s var(--ease), opacity .15s; }
.launcher[aria-expanded="true"] .l-label { max-width: 0; opacity: 0; }
@keyframes launcher-in { from { opacity: 0; transform: translateY(8px) scale(.9); } to { opacity: 1; transform: none; } }
.badge {
  position: absolute; top: -5px; right: -5px; min-width: 20px; height: 20px; padding: 0 6px; border-radius: 999px;
  background: var(--danger); color: #fff; font-size: 11px; font-weight: 600; line-height: 20px; text-align: center;
  font-variant-numeric: tabular-nums; box-shadow: 0 0 0 2px var(--bg); animation: pop .24s var(--ease) both;
}
.pos-left .badge { right: auto; left: -5px; }
@keyframes pop { 0% { transform: scale(.5); opacity: 0; } 60% { transform: scale(1.12); opacity: 1; } 100% { transform: none; } }

/* Panel: rises from the launcher's corner, eases out on close. */
.panel {
  position: fixed; right: var(--ox, 20px); bottom: var(--gap-launcher); width: 384px;
  height: min(640px, calc(100vh - var(--gap-launcher) - 24px));
  display: flex; flex-direction: column; overflow: hidden; pointer-events: auto;
  background: var(--bg); border-radius: var(--r-panel, 16px); box-shadow: var(--shadow);
  transform-origin: bottom right; animation: panel-in .24s var(--ease) both;
}
.pos-left .panel { right: auto; left: var(--ox, 20px); transform-origin: bottom left; }
.preview .panel { height: min(640px, calc(100% - var(--gap-launcher) - 16px)); max-width: calc(100% - 2 * var(--ox, 20px)); }
.panel.leaving { animation: panel-out .16s cubic-bezier(.4, 0, 1, 1) both; pointer-events: none; }
@keyframes panel-in { from { opacity: 0; transform: translateY(12px) scale(.96); } to { opacity: 1; transform: none; } }
@keyframes panel-out { from { opacity: 1; transform: none; } to { opacity: 0; transform: translateY(8px) scale(.98); } }
@media (max-width: 479px) {
  .root:not(.preview) .panel { inset: 0; width: auto; height: 100%; border-radius: 0; transform-origin: bottom center; }
  .root:not(.preview) .launcher.hide-mobile { display: none; }
}

/* Headers */
.head { padding: 12px 12px 11px; display: flex; align-items: center; gap: 10px; border-bottom: 1px solid var(--line); background: var(--bg); flex: none; }
.head.home {
  --head-bg: var(--bg-3);
  padding: 16px 16px 20px; flex-direction: column; align-items: stretch; gap: 0;
  background: linear-gradient(180deg, color-mix(in srgb, var(--accent) 7%, var(--bg-3)) 0%, var(--bg-3) 100%);
}
.head-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.head-main { flex: 1; min-width: 0; }
.ws { display: flex; align-items: center; gap: 9px; min-width: 0; font-size: 13.5px; font-weight: 600; letter-spacing: -.01em; }
.ws-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ws-glyph { width: 26px; height: 26px; flex: none; border-radius: var(--r-av, 6px); overflow: hidden; background: var(--accent); color: var(--accent-fg); display: grid; place-items: center; font-size: 12px; font-weight: 700; }
.ws-glyph img { width: 100%; height: 100%; object-fit: cover; }
.greet { margin-top: 18px; font-size: 22px; line-height: 28px; font-weight: 600; letter-spacing: -.025em; white-space: pre-line; animation: rise .22s .03s var(--ease) both; }
.sub-row { margin-top: 8px; display: flex; align-items: center; gap: 8px; animation: rise .22s .06s var(--ease) both; }
.sub { color: var(--fg-3); font-size: 13px; line-height: 18px; }
.title { font-size: 14px; font-weight: 600; letter-spacing: -.01em; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.title-sub { font-size: 12px; color: var(--fg-3); margin-top: 1px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.icon-btn { width: 30px; height: 30px; flex: none; border-radius: var(--r-ctl, 8px); display: grid; place-items: center; color: var(--fg-3); transition: background .12s, color .12s, transform .12s; }
.icon-btn:hover { background: var(--bg-2); color: var(--fg); }
.icon-btn:active { transform: scale(.92); }
.icon-btn:disabled { opacity: .4; cursor: default; background: none; }

/* Avatars */
.av { width: 18px; height: 18px; flex: none; border-radius: var(--r-av, 6px); background: var(--bg-2); color: var(--fg-2); font-size: 8.5px; font-weight: 600; display: grid; place-items: center; overflow: hidden; }
.av img { width: 100%; height: 100%; object-fit: cover; }
.bot-av { background: var(--ai-soft); }
.bot-av.is-ollie { background: transparent; overflow: visible; }
.stack { display: inline-flex; align-items: center; flex: none; }
.stack .av { box-shadow: 0 0 0 2px var(--head-bg, var(--bg)); }
.stack .av + .av { margin-inline-start: -7px; }
.stack .bot-av.is-ollie { box-shadow: none; }
.ollie-eyes { transform-box: fill-box; transform-origin: center; animation: blink 5.2s ease-in-out infinite; }
@keyframes blink { 0%, 95%, 100% { transform: none; } 97% { transform: scaleY(.15); } }

/* Home */
.body { position: relative; flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; scrollbar-width: thin; }
.home-body { padding: 16px; display: flex; flex-direction: column; gap: 12px; }
.card { border: 1px solid var(--line); border-radius: var(--r-card, 10px); background: var(--bg); overflow: hidden; animation: rise .22s .06s var(--ease) both; }
.card-h { padding: 12px 14px 6px; font-size: 11px; font-weight: 600; letter-spacing: .06em; text-transform: uppercase; color: var(--fg-4); }
.conv { width: 100%; display: flex; gap: 10px; align-items: flex-start; padding: 10px 14px; text-align: start; border-top: 1px solid var(--line); transition: background .12s; }
.card-h + .conv { border-top: 0; }
.conv:hover { background: var(--bg-3); }
.conv-main { flex: 1; min-width: 0; }
.conv-top { display: flex; justify-content: space-between; gap: 8px; }
.conv-subject { font-size: 13px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.conv-prev { display: block; font-size: 12.5px; color: var(--fg-3); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-top: 1px; }
.time { font-family: var(--mono); font-size: 10.5px; color: var(--fg-4); flex: none; }
.dot { width: 7px; height: 7px; border-radius: 99px; margin-top: 6px; flex: none; background: var(--line-2); }
.dot.open { background: var(--accent); }
.primary {
  display: flex; align-items: center; justify-content: center; gap: 8px; height: 40px; border-radius: var(--r-ctl, 8px); width: 100%;
  background: var(--accent); color: var(--accent-fg); font-size: 13.5px; font-weight: 500; transition: opacity .12s, transform .12s, box-shadow .15s;
}
.primary:hover { opacity: .92; }
.primary:active { transform: scale(.985); }
.primary:disabled { opacity: .5; cursor: default; }
.primary.start { height: 44px; justify-content: space-between; padding: 0 16px; box-shadow: 0 1px 2px rgb(0 0 0 / .08); animation: rise .2s var(--ease) both; }
.start-ic { display: grid; place-items: center; transition: transform .18s var(--ease); }
.start:hover .start-ic { transform: translateX(2px); }
[dir="rtl"] .start:hover .start-ic { transform: translateX(-2px); }
.suggest { display: flex; flex-wrap: wrap; gap: 6px; }
.sugg {
  min-height: 32px; padding: 6px 12px; border-radius: var(--r-ctl, 8px); border: 1px solid var(--line); background: var(--bg);
  font-size: 13px; line-height: 18px; text-align: start; color: var(--fg);
  transition: border-color .12s, background .12s, transform .12s var(--ease), box-shadow .12s;
  animation: rise .2s calc(var(--i, 0) * 30ms + 40ms) var(--ease) both;
}
.sugg:hover { border-color: var(--line-2); background: var(--bg-3); transform: translateY(-1px); box-shadow: 0 2px 6px -2px rgb(0 0 0 / .08); }
.sugg:active { transform: scale(.97); transition-duration: .06s; }
.sugg:disabled { opacity: .5; cursor: default; transform: none; }
.link-row { display: flex; align-items: center; justify-content: space-between; padding: 12px 14px; font-size: 13px; font-weight: 500; color: var(--fg); text-decoration: none; transition: background .12s; }
.link-row:hover { background: var(--bg-3); }
.link-row svg { color: var(--fg-3); transition: transform .16s var(--ease); }
.link-row:hover svg { transform: translate(1px, -1px); }

/* Thread */
.body-inner { min-height: 100%; display: flex; flex-direction: column; }
.thread { padding: 16px 14px 8px; display: flex; flex-direction: column; gap: 2px; }
.day { align-self: center; font-family: var(--mono); font-size: 10.5px; color: var(--fg-4); margin: 10px 0 6px; }
.row { display: flex; flex-direction: column; max-width: 84%; animation: msg-in .2s var(--ease) both; }
.row.me { align-self: flex-end; align-items: flex-end; transform-origin: bottom right; }
.row.them { align-self: flex-start; align-items: flex-start; transform-origin: bottom left; }
[dir="rtl"] .row.me { transform-origin: bottom left; } [dir="rtl"] .row.them { transform-origin: bottom right; }
.row.no-enter { animation: none; }
.row.gap { margin-top: 12px; }
.row.gap:first-child, .day + .row.gap { margin-top: 0; }
@keyframes msg-in { from { opacity: 0; transform: translateY(6px) scale(.98); } to { opacity: 1; transform: none; } }
@keyframes rise { from { opacity: 0; transform: translateY(5px); } to { opacity: 1; transform: none; } }
.who { font-size: 11.5px; color: var(--fg-3); margin: 0 2px 4px; display: flex; align-items: center; gap: 6px; }
.who-name { font-weight: 500; color: var(--fg-2); }
.msg { padding: 8px 12px; border-radius: var(--r-bubble, 14px); font-size: 13.5px; line-height: 19.5px; overflow-wrap: anywhere; max-width: 100%; }
.me .msg { background: var(--accent); color: var(--accent-fg); border-end-end-radius: 4px; white-space: pre-wrap; transition: opacity .15s; }
.is-pending .msg { opacity: .72; }
.them .msg { background: var(--bg-2); color: var(--fg); border-end-start-radius: 4px; }
.row.me:not(.gap) .msg { border-start-end-radius: 4px; }
.row.them:not(.gap) .msg { border-start-start-radius: 4px; }
.msg p + p, .msg p + ul, .msg p + ol, .msg ul + p, .msg ol + p, .msg pre, .msg blockquote { margin-top: 6px; }
.msg ul, .msg ol { padding-inline-start: 18px; }
.msg li + li { margin-top: 2px; }
.msg a { color: inherit; text-decoration: underline; text-underline-offset: 2px; text-decoration-color: color-mix(in srgb, currentColor 40%, transparent); }
.msg a:hover { text-decoration-color: currentColor; }
.msg code { font-family: var(--mono); font-size: 12px; padding: 0 3px; border-radius: 3px; background: color-mix(in srgb, var(--fg) 7%, transparent); }
.msg pre { font-family: var(--mono); font-size: 12px; white-space: pre-wrap; }
.msg pre code { padding: 0; background: none; }
.msg blockquote { border-inline-start: 2px solid var(--line-2); padding-inline-start: 8px; color: var(--fg-2); }
.meta { font-family: var(--mono); font-size: 10px; color: var(--fg-4); margin: 3px 4px 0; display: flex; gap: 6px; align-items: center; }
.meta.failed { color: var(--danger); font-family: var(--font); font-size: 11.5px; }
.meta button { text-decoration: underline; color: inherit; font-size: inherit; }
.files { display: flex; flex-direction: column; gap: 4px; margin-top: 4px; }
.me .files { align-items: flex-end; }
.thumb { display: block; max-width: 220px; max-height: 160px; border-radius: var(--r-card, 10px); border: 1px solid var(--line); object-fit: cover; }
.chip { display: inline-flex; align-items: center; gap: 6px; max-width: 240px; height: 30px; padding: 0 10px; border-radius: var(--r-ctl, 8px); border: 1px solid var(--line); background: var(--bg); font-size: 12px; color: var(--fg); text-decoration: none; transition: border-color .12s; }
a.chip:hover { border-color: var(--line-2); }
.chip span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.chip small { color: var(--fg-4); font-family: var(--mono); font-size: 10.5px; flex: none; }

/* The agent: violet kept to its avatar, the AI pill, tool steps and the caret. */
.ai .msg { background: var(--bg-3); border: 1px solid var(--line); }
.ai-pill { display: inline-flex; align-items: center; height: 16px; padding: 0 5px; border-radius: 4px; background: var(--ai-soft); color: var(--ai); font-size: 9.5px; font-weight: 600; letter-spacing: .04em; }
.live .msg { min-width: 52px; transition: min-height .2s var(--ease); }
.live.leaving { animation: fade-out .2s ease-out forwards; }
@keyframes fade-out { to { opacity: 0; transform: translateY(-2px); } }
.typing-dots { display: inline-flex; align-items: center; height: 19.5px; color: var(--ai); }
.md.writing > p:last-child::after, .md.writing > ul:last-child > li:last-child::after, .md.writing > ol:last-child > li:last-child::after, .md.writing > blockquote:last-child::after {
  content: ""; display: inline-block; width: 2px; height: 14px; margin-inline-start: 2px; vertical-align: -2px; border-radius: 1px;
  background: var(--ai); animation: caret 1s ease-in-out infinite;
}
@keyframes caret { 0%, 100% { opacity: 1; } 50% { opacity: .25; } }
.tools { display: flex; flex-direction: column; gap: 4px; }
.tools + .md, .tools + .typing-dots { margin-top: 6px; }
.tools + .typing-dots { display: flex; }
.tool { display: flex; align-items: center; gap: 7px; font-size: 12px; line-height: 18px; color: var(--fg-3); animation: rise .18s var(--ease) both; }
.tool-ic { width: 16px; height: 16px; flex: none; display: grid; place-items: center; border-radius: 99px; color: var(--ai); background: var(--ai-soft); transition: background .2s, color .2s; }
.t-done .tool-ic { color: var(--ok); background: var(--ok-soft); }
.t-done .tool-ic svg, .t-error .tool-ic svg { animation: pop .22s var(--ease) both; }
.t-error .tool-ic { color: var(--fg-3); background: var(--bg-2); }
.t-running span:last-child {
  background: linear-gradient(90deg, var(--fg-3) 0%, var(--fg-3) 35%, var(--fg) 50%, var(--fg-3) 65%, var(--fg-3) 100%) 0 0 / 250% 100%;
  -webkit-background-clip: text; background-clip: text; -webkit-text-fill-color: transparent; animation: shimmer 1.8s linear infinite;
}
[dir="rtl"] .t-running span:last-child { animation-direction: reverse; }
@keyframes shimmer { from { background-position: 100% 0; } to { background-position: 0 0; } }
.spin { animation: spin .7s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

/* Sources */
.sources { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 6px; }
.src {
  display: inline-flex; align-items: center; gap: 6px; max-width: 240px; height: 26px; padding: 0 10px 0 8px; border-radius: var(--r-ctl, 8px);
  border: 1px solid var(--line); background: var(--bg); color: var(--fg-2); font-size: 12px; text-decoration: none;
  transition: border-color .12s, color .12s, background .12s, transform .12s var(--ease);
}
.src svg { color: var(--ai); flex: none; }
.src span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.src:hover { border-color: var(--ai-line); color: var(--fg); background: var(--ai-soft); }
.src:active { transform: scale(.97); }

/* Action cards */
.action {
  margin-top: 6px; width: 272px; max-width: 100%; padding: 11px 12px 12px; border-radius: var(--r-card, 10px);
  border: 1px solid var(--line); background: var(--bg); box-shadow: 0 1px 2px rgb(0 0 0 / .04);
  transition: border-color .2s, background .2s; animation: rise .2s var(--ease) both;
}
.action.s-pending { border-color: var(--ai-line); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ai-soft) 70%, transparent); }
.action-top { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-height: 20px; }
.action-tool { display: inline-flex; align-items: center; gap: 5px; min-width: 0; font-size: 11.5px; font-weight: 500; color: var(--ai); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.action-need { font-size: 11px; color: var(--fg-4); white-space: nowrap; }
.action-title { margin-top: 6px; font-size: 13.5px; line-height: 19px; font-weight: 600; letter-spacing: -.01em; overflow-wrap: anywhere; }
.action-sum { margin-top: 2px; font-size: 12.5px; line-height: 18px; color: var(--fg-2); overflow-wrap: anywhere; }
.action-btns { display: grid; grid-template-rows: 0fr; opacity: 0; transition: grid-template-rows .22s var(--ease), opacity .15s, margin-top .22s var(--ease); }
.action-btns[data-open] { grid-template-rows: 1fr; opacity: 1; margin-top: 10px; }
.action-btns > div { min-height: 0; overflow: hidden; display: flex; gap: 6px; justify-content: flex-end; }
.action-btns:not([data-open]) button { visibility: hidden; }
.btn {
  height: 30px; padding: 0 12px; border-radius: var(--r-ctl, 8px); display: inline-flex; align-items: center; justify-content: center; gap: 6px;
  font-size: 12.5px; font-weight: 500; transition: background .12s, border-color .12s, opacity .12s, transform .12s var(--ease);
}
.btn:active:not(:disabled) { transform: scale(.96); }
.btn:disabled { cursor: default; }
.btn.solid { background: var(--accent); color: var(--accent-fg); }
.btn.solid:hover:not(:disabled) { opacity: .9; }
.btn.ghost { border: 1px solid var(--line-2); color: var(--fg); }
.btn.ghost:hover:not(:disabled) { background: var(--bg-2); }
.btn:disabled:not([aria-busy="true"]) { opacity: .5; }
.pill { display: inline-flex; align-items: center; gap: 4px; height: 20px; padding: 0 8px; border-radius: 999px; font-size: 11px; font-weight: 500; white-space: nowrap; animation: pop .22s var(--ease) both; }
.p-confirmed { background: var(--ok-soft); color: var(--ok); }
.p-declined, .p-expired { background: var(--bg-2); color: var(--fg-3); }
.p-failed { background: var(--danger-soft); color: var(--danger); }

/* Ratings: quiet until hover or focus; always there on touch screens. */
.meta-row { display: flex; align-items: center; gap: 4px; min-height: 22px; margin-top: 2px; }
.meta-row .meta { margin-top: 0; }
.rate { display: inline-flex; gap: 1px; opacity: 0; transition: opacity .15s; }
.row.ai:hover .rate, .row.ai:focus-within .rate, .rate.rated { opacity: 1; }
@media (hover: none) { .rate { opacity: 1; } }
.rate-btn { width: 24px; height: 22px; display: grid; place-items: center; border-radius: 6px; color: var(--fg-4); transition: color .12s, background .12s, transform .12s var(--ease); }
.rate-btn:hover { color: var(--fg-2); background: var(--bg-2); }
.rate-btn:active { transform: scale(.88); }
.rate-btn.on { color: var(--ai); background: var(--ai-soft); }
.rate-btn.on svg { animation: pop .24s var(--ease) both; }

/* Handoff */
.offer { margin-top: 6px; display: flex; flex-direction: column; align-items: flex-start; }
.handoff {
  display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 11px; border-radius: var(--r-ctl, 8px);
  border: 1px solid var(--line-2); background: var(--bg); font-size: 12.5px; font-weight: 500; color: var(--fg);
  transition: background .12s, border-color .12s, opacity .12s, transform .12s; animation: rise .2s .08s var(--ease) both;
}
.handoff:hover { background: var(--bg-2); }
.handoff:active { transform: scale(.97); }
.handoff:disabled { opacity: .6; cursor: default; }
.handed { margin: 6px 14px 14px; display: flex; align-items: flex-start; gap: 10px; padding: 11px 12px; border-radius: var(--r-card, 10px); background: var(--bg-3); border: 1px solid var(--line); animation: rise .22s var(--ease) both; }
.handed-ic { width: 28px; height: 28px; flex: none; display: grid; place-items: center; border-radius: var(--r-av, 6px); background: var(--bg); border: 1px solid var(--line); color: var(--fg-2); }
.handed-text { display: flex; flex-direction: column; gap: 1px; font-size: 12px; line-height: 17px; color: var(--fg-3); }
.handed-text b { font-size: 13px; line-height: 18px; font-weight: 600; color: var(--fg); }

/* A person typing */
.typing { align-self: flex-start; display: flex; align-items: center; gap: 8px; padding: 2px 14px 12px; color: var(--fg-3); font-size: 12px; animation: rise .18s var(--ease) both; }
.typing .dots { padding: 11px 12px; border-radius: var(--r-bubble, 14px); border-end-start-radius: 4px; background: var(--bg-2); }
.dots { display: inline-flex; align-items: center; gap: 3px; }
.dots span { width: 5px; height: 5px; border-radius: 99px; background: currentColor; opacity: .35; animation: wave 1.1s infinite ease-in-out; }
.dots span:nth-child(2) { animation-delay: .14s; } .dots span:nth-child(3) { animation-delay: .28s; }
@keyframes wave { 0%, 70%, 100% { opacity: .35; transform: none; } 35% { opacity: 1; transform: translateY(-2px); } }

.jump {
  position: sticky; bottom: 10px; margin: -42px auto 10px; width: 32px; height: 32px; display: grid; place-items: center; z-index: 2;
  border-radius: 999px; background: var(--bg); color: var(--fg-2); border: 1px solid var(--line); box-shadow: 0 4px 12px -4px rgb(0 0 0 / .18);
  animation: pop .2s var(--ease) both; transition: color .12s, transform .12s;
}
.jump:hover { color: var(--fg); transform: translateY(1px); }

.empty { padding: 32px 24px; text-align: center; color: var(--fg-3); font-size: 13px; animation: rise .22s var(--ease) both; }
.empty b { display: block; color: var(--fg); font-weight: 600; font-size: 14px; margin-bottom: 4px; }

/* Composer: one inset field with its actions inside. */
.composer { padding: 10px 12px 12px; display: flex; flex-direction: column; gap: 6px; background: var(--bg); flex: none; }
.pending-files { display: flex; flex-wrap: wrap; gap: 6px; }
.pending-files .chip { height: 26px; animation: rise .16s var(--ease) both; }
.pending-files .chip button { color: var(--fg-3); display: grid; place-items: center; border-radius: 3px; }
.pending-files .chip button:hover { color: var(--fg); }
.cbar {
  display: flex; align-items: flex-end; gap: 4px; padding: 5px; padding-inline-start: 12px;
  border: 1px solid var(--line); border-radius: var(--r-bar, 10px); background: var(--bg);
  transition: border-color .15s, box-shadow .15s;
}
.cbar:hover { border-color: var(--line-2); }
.cbar:focus-within { border-color: var(--fg-4); box-shadow: 0 0 0 3px var(--ring); }
.cbar.off { background: var(--bg-3); }
.cbar .icon-btn { width: 30px; height: 30px; }
.input-wrap { position: relative; flex: 1; min-width: 0; display: flex; }
textarea.input {
  flex: 1; width: 100%; resize: none; border: 0; background: transparent; font-size: 14px; line-height: 20px; padding: 5px 0;
  max-height: 120px; min-height: 30px; color: var(--fg); transition: height .12s var(--ease); scrollbar-width: thin;
}
textarea.mirror { position: absolute; inset-inline: 0; top: 0; height: 0 !important; min-height: 0; visibility: hidden; pointer-events: none; overflow: hidden; transition: none; }
textarea.input:disabled { cursor: not-allowed; }
textarea.input::placeholder, input.field::placeholder { color: var(--fg-4); }
.send {
  width: 30px; height: 30px; flex: none; border-radius: var(--r-ctl, 8px); display: grid; place-items: center;
  background: var(--accent); color: var(--accent-fg); opacity: .28; transform: scale(.9);
  transition: opacity .16s, transform .18s var(--ease);
}
.send.ready { opacity: 1; transform: none; }
.send.ready:hover { transform: translateY(-1px); }
.send.ready:active { transform: scale(.9); transition-duration: .06s; }
.send:disabled { cursor: default; }
.drop { position: absolute; inset: 0; z-index: 3; display: grid; place-items: center; background: color-mix(in srgb, var(--bg) 88%, transparent); border: 1.5px dashed var(--line-2); border-radius: inherit; font-size: 13px; font-weight: 500; color: var(--fg-2); animation: fade .12s ease-out; }
@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
.capture { margin: 12px 14px 4px; padding: 14px; border: 1px solid var(--line); border-radius: var(--r-card, 10px); display: flex; flex-direction: column; gap: 8px; background: var(--bg-3); animation: rise .2s var(--ease) both; }
.capture p { font-size: 13px; color: var(--fg-2); }
input.field { height: 36px; border: 1px solid var(--line); border-radius: var(--r-ctl, 8px); padding: 0 12px; background: var(--bg); font-size: 13.5px; color: var(--fg); transition: border-color .15s, box-shadow .15s; }
input.field:hover { border-color: var(--line-2); }
input.field:focus { border-color: var(--fg-4); box-shadow: 0 0 0 3px var(--ring); }
.err { color: var(--danger); font-size: 12px; margin-top: 6px; }
.foot { display: flex; align-items: center; justify-content: center; gap: 5px; height: 30px; font-size: 11px; color: var(--fg-4); border-top: 1px solid var(--line); text-decoration: none; flex: none; transition: color .12s; }
.foot:hover { color: var(--fg-2); }
[dir="rtl"] .flip svg { transform: scaleX(-1); }
[dir="rtl"] .link-row:hover .flip svg { transform: scaleX(-1) translate(1px, -1px); }
.time, .day, .meta { unicode-bidi: plaintext; }
.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.skel { height: 12px; border-radius: 4px; margin: 10px 14px; background: linear-gradient(90deg, var(--bg-2) 0%, var(--bg-3) 50%, var(--bg-2) 100%) 0 0 / 200% 100%; animation: skel 1.4s ease-in-out infinite; }
@keyframes skel { from { background-position: 100% 0; } to { background-position: -100% 0; } }

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { animation: none !important; transition: none !important; scroll-behavior: auto !important; }
  .t-running span:last-child { -webkit-text-fill-color: currentColor; background: none; }
  .dots span { opacity: .55; }
}
`, tn = 0;
Array.isArray;
function Q(t, n, r, i, a, o) {
	n || (n = {});
	var s, c, l = n;
	if ("ref" in l && typeof t != "function") for (c in l = {}, n) c == "ref" ? s = n[c] : l[c] = n[c];
	var u = {
		type: t,
		props: l,
		key: r,
		ref: s,
		__k: null,
		__: null,
		__b: 0,
		__e: null,
		__c: null,
		constructor: void 0,
		__v: --tn,
		__i: -1,
		__u: 0
	};
	return (a || o) && (u.__source = a, u.__self = o), e.vnode && e.vnode(u), u;
}
//#endregion
//#region widget/src/icons.tsx
var $ = (e, t = 16, n = 1.75) => /* @__PURE__ */ Q("svg", {
	width: t,
	height: t,
	viewBox: "0 0 24 24",
	fill: "none",
	stroke: "currentColor",
	"stroke-width": n,
	"stroke-linecap": "round",
	"stroke-linejoin": "round",
	"aria-hidden": "true",
	children: e
}), nn = (e = 16) => $(/* @__PURE__ */ Q("path", { d: "M18 6 6 18M6 6l12 12" }), e), rn = () => $(/* @__PURE__ */ Q("path", { d: "m15 18-6-6 6-6" })), an = () => $(/* @__PURE__ */ Q("path", { d: "m21.4 11.1-9.2 9.2a6 6 0 0 1-8.5-8.5l9.2-9.2a4 4 0 0 1 5.7 5.7l-9.2 9.2a2 2 0 0 1-2.8-2.8l8.5-8.5" })), on = () => $(/* @__PURE__ */ Q("path", { d: "M12 19V5M5 12l7-7 7 7" }), 16, 2), sn = () => $(/* @__PURE__ */ Q("path", { d: "M12 5v14M19 12l-7 7-7-7" }), 14, 2), cn = () => $(/* @__PURE__ */ Q("path", { d: "M7 17 17 7M8 7h9v9" }), 14), ln = () => $(/* @__PURE__ */ Q("path", { d: "M5 12h14M13 6l6 6-6 6" }), 15), un = () => $(/* @__PURE__ */ Q("path", { d: "M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8zM14 3v5h5" }), 14), dn = () => $(/* @__PURE__ */ Q("path", { d: "M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1" }), 12, 2), fn = (e = 14) => $(/* @__PURE__ */ Q(y, { children: [/* @__PURE__ */ Q("circle", {
	cx: "12",
	cy: "8",
	r: "3.5"
}), /* @__PURE__ */ Q("path", { d: "M5 20a7 7 0 0 1 14 0" })] }), e), pn = (e = 14) => $(/* @__PURE__ */ Q("path", { d: "M20 6 9 17l-5-5" }), e, 2.25), mn = (e = 12) => $(/* @__PURE__ */ Q("path", { d: "M17 7 7 17M7 7l10 10" }), e, 2.25), hn = (e = 12) => $(/* @__PURE__ */ Q(y, { children: [/* @__PURE__ */ Q("circle", {
	cx: "12",
	cy: "12",
	r: "8.5"
}), /* @__PURE__ */ Q("path", { d: "M12 7.5V12l3 2" })] }), e, 2), gn = (e = 12) => $(/* @__PURE__ */ Q("path", { d: "M13 3 5 13.5h6L10 21l8-10.5h-6L13 3Z" }), e, 2), _n = () => $(/* @__PURE__ */ Q("path", { d: "M7 10v10M7 10l4-7a2 2 0 0 1 3 2l-1 4h5.5a2 2 0 0 1 2 2.4l-1.3 6.4a2 2 0 0 1-2 1.6H7m0-9.4H4.5a1 1 0 0 0-1 1V19a1 1 0 0 0 1 1H7" }), 14), vn = () => $(/* @__PURE__ */ Q("path", { d: "M17 14V4m0 10-4 7a2 2 0 0 1-3-2l1-4H5.5a2 2 0 0 1-2-2.4l1.3-6.4a2 2 0 0 1 2-1.6H17m0 9.4h2.5a1 1 0 0 0 1-1V5a1 1 0 0 0-1-1H17" }), 14), yn = (e = 24) => /* @__PURE__ */ Q("svg", {
	width: e,
	height: e,
	viewBox: "0 0 24 24",
	fill: "none",
	"aria-hidden": "true",
	children: /* @__PURE__ */ Q("path", {
		fill: "currentColor",
		d: "M12 3.5c-4.9 0-8.75 3.43-8.75 7.75 0 2.3 1.1 4.36 2.86 5.78-.13 1.12-.6 2.2-1.37 3.08a.5.5 0 0 0 .44.83c1.9-.2 3.47-.86 4.62-1.67.7.15 1.45.23 2.2.23 4.9 0 8.75-3.43 8.75-7.75S16.9 3.5 12 3.5Z"
	})
}), bn = () => /* @__PURE__ */ Q("svg", {
	class: "spin",
	width: "12",
	height: "12",
	viewBox: "0 0 24 24",
	fill: "none",
	"aria-hidden": "true",
	children: [/* @__PURE__ */ Q("circle", {
		cx: "12",
		cy: "12",
		r: "9",
		stroke: "currentColor",
		"stroke-opacity": ".22",
		"stroke-width": "3"
	}), /* @__PURE__ */ Q("path", {
		d: "M21 12a9 9 0 0 0-9-9",
		stroke: "currentColor",
		"stroke-width": "3",
		"stroke-linecap": "round"
	})]
}), xn = ({ s: e = 10 }) => /* @__PURE__ */ Q("svg", {
	width: e,
	height: e,
	viewBox: "0 0 64 64",
	"aria-hidden": "true",
	children: /* @__PURE__ */ Q("path", {
		fill: "currentColor",
		d: "M11 4h12a7 7 0 0 1 7 7v12a7 7 0 0 1-7 7h-12a7 7 0 0 1-7-7v-12a7 7 0 0 1 7-7Z M47 4a13 13 0 1 1 0 26a13 13 0 1 1 0-26Z M11 34h12a7 7 0 0 1 7 7v12a7 7 0 0 1-7 7h-12a7 7 0 0 1-7-7v-12a7 7 0 0 1 7-7Z M41 34h12a7 7 0 0 1 7 7v12a7 7 0 0 1-7 7h-12a7 7 0 0 1-7-7v-12a7 7 0 0 1 7-7Z"
	})
}), Sn = ({ s: e = 18 }) => /* @__PURE__ */ Q("svg", {
	class: "ollie",
	width: e,
	height: e,
	viewBox: "0 0 24 24",
	"aria-hidden": "true",
	children: [
		/* @__PURE__ */ Q("defs", { children: /* @__PURE__ */ Q("radialGradient", {
			id: "od-ollie",
			cx: "36%",
			cy: "30%",
			r: "75%",
			children: [
				/* @__PURE__ */ Q("stop", {
					offset: "0",
					"stop-color": "#d9d0ff"
				}),
				/* @__PURE__ */ Q("stop", {
					offset: ".5",
					"stop-color": "#7d5af0"
				}),
				/* @__PURE__ */ Q("stop", {
					offset: "1",
					"stop-color": "#3f22a6"
				})
			]
		}) }),
		/* @__PURE__ */ Q("path", {
			fill: "url(#od-ollie)",
			d: "M12 2.4c5.4 0 9.5 3.8 9.5 9.1 0 5.6-4.1 10.1-9.6 10.1-5.3 0-9.4-4.3-9.4-9.8 0-5.4 4.1-9.4 9.5-9.4Z"
		}),
		/* @__PURE__ */ Q("ellipse", {
			cx: "8.4",
			cy: "7.2",
			rx: "2.7",
			ry: "1.5",
			fill: "#fff",
			opacity: ".6",
			transform: "rotate(-24 8.4 7.2)"
		}),
		/* @__PURE__ */ Q("g", {
			class: "ollie-eyes",
			fill: "#fff",
			children: [/* @__PURE__ */ Q("rect", {
				x: "9.1",
				y: "10.6",
				width: "1.7",
				height: "2.8",
				rx: ".85"
			}), /* @__PURE__ */ Q("rect", {
				x: "13.3",
				y: "10.6",
				width: "1.7",
				height: "2.8",
				rx: ".85"
			})]
		})
	]
}), Cn = () => typeof matchMedia == "function" && matchMedia("(prefers-reduced-motion: reduce)").matches, wn = () => /* @__PURE__ */ Q("span", {
	class: "dots",
	"aria-hidden": "true",
	children: [
		/* @__PURE__ */ Q("span", {}),
		/* @__PURE__ */ Q("span", {}),
		/* @__PURE__ */ Q("span", {})
	]
});
function Tn({ bot: e, size: t = 18 }) {
	return /* @__PURE__ */ Q("span", {
		class: `av bot-av${e.avatar_url ? "" : " is-ollie"}`,
		style: {
			width: `${t}px`,
			height: `${t}px`
		},
		"aria-hidden": "true",
		children: e.avatar_url ? /* @__PURE__ */ Q("img", {
			src: e.avatar_url,
			alt: "",
			referrerpolicy: "no-referrer"
		}) : /* @__PURE__ */ Q(Sn, { s: t })
	});
}
function En({ p: e, size: t = 18, one: n }) {
	return /* @__PURE__ */ Q("span", {
		class: "av",
		style: {
			width: `${t}px`,
			height: `${t}px`,
			fontSize: `${Math.round(t * .42)}px`
		},
		"aria-hidden": "true",
		children: e.avatar_url ? /* @__PURE__ */ Q("img", {
			src: e.avatar_url,
			alt: "",
			referrerpolicy: "no-referrer"
		}) : n ? e.name.trim().charAt(0).toUpperCase() || "?" : se(e.name)
	});
}
function Dn({ team: e, bot: t, size: n = 24 }) {
	return !e.length && !t ? null : /* @__PURE__ */ Q("span", {
		class: "stack",
		"aria-hidden": "true",
		children: [t && /* @__PURE__ */ Q(Tn, {
			bot: t,
			size: n
		}), e.slice(0, 3).map((e, t) => /* @__PURE__ */ Q(En, {
			p: e,
			size: n,
			one: !0
		}, `${e.name}-${t}`))]
	});
}
function On({ bot: e }) {
	return /* @__PURE__ */ Q("div", {
		class: "who",
		children: [
			/* @__PURE__ */ Q(Tn, { bot: e }),
			/* @__PURE__ */ Q("span", {
				class: "who-name",
				children: e.name
			}),
			/* @__PURE__ */ Q("span", {
				class: "ai-pill",
				title: H("aiAnswer"),
				children: [/* @__PURE__ */ Q("span", {
					class: "sr",
					children: H("aiAnswer")
				}), /* @__PURE__ */ Q("span", {
					"aria-hidden": "true",
					children: H("ai")
				})]
			})
		]
	});
}
function kn({ m: e, me: t, grouped: n, onRetry: r }) {
	var i, a, o;
	let s = (i = (a = e.author.user) == null ? void 0 : a.name) == null ? e.author.type === "system" ? H("automated") : H("support") : i, c = t ? !!e.body_text : !!(e.body_html || e.body_text);
	return /* @__PURE__ */ Q("div", {
		class: `row ${t ? "me" : "them"}${n ? "" : " gap"}${e.local === "pending" ? " is-pending" : ""}`,
		children: [
			!t && !n && /* @__PURE__ */ Q("div", {
				class: "who",
				children: [/* @__PURE__ */ Q(En, { p: {
					name: s,
					avatar_url: ((o = e.author.user) == null ? void 0 : o.avatar_url) || null
				} }), /* @__PURE__ */ Q("span", {
					class: "who-name",
					children: s
				})]
			}),
			c && (t || !e.body_html ? /* @__PURE__ */ Q("div", {
				class: "msg",
				dir: "auto",
				children: e.body_text
			}) : /* @__PURE__ */ Q("div", {
				class: "msg",
				dangerouslySetInnerHTML: { __html: z(e.body_html) }
			})),
			e.attachments.length > 0 && /* @__PURE__ */ Q("div", {
				class: "files",
				children: e.attachments.map((e) => /* @__PURE__ */ Q(An, { a: e }, e.id))
			}),
			e.local === "failed" ? /* @__PURE__ */ Q("div", {
				class: "meta failed",
				children: [
					H("notSent"),
					" ",
					/* @__PURE__ */ Q("button", {
						onClick: r,
						children: H("retry")
					})
				]
			}) : e.local === "pending" ? /* @__PURE__ */ Q("div", {
				class: "meta",
				children: H("sending")
			}) : !n || t ? /* @__PURE__ */ Q("div", {
				class: "meta",
				children: Ie(e.created_at)
			}) : null
		]
	});
}
function An({ a: e }) {
	return e.content_type.startsWith("image/") && e.url ? /* @__PURE__ */ Q("a", {
		href: e.url,
		target: "_blank",
		rel: "noopener noreferrer",
		children: /* @__PURE__ */ Q("img", {
			class: "thumb",
			src: e.url,
			alt: e.filename,
			loading: "lazy"
		})
	}) : /* @__PURE__ */ Q("a", {
		class: "chip",
		href: e.url || void 0,
		target: "_blank",
		rel: "noopener noreferrer",
		children: [
			/* @__PURE__ */ Q(un, {}),
			/* @__PURE__ */ Q("span", { children: e.filename }),
			/* @__PURE__ */ Q("small", { children: oe(e.size_bytes) })
		]
	});
}
function jn({ c: e, m: t, convId: n, bot: r, offer: i, swapped: a, rating: o, action: s }) {
	let c = Et(() => t.body_html ? ce(z(t.body_html)) : null, [t.body_html]);
	return /* @__PURE__ */ Q("div", {
		class: `row them gap ai${a ? " no-enter" : ""}`,
		children: [
			/* @__PURE__ */ Q(On, { bot: r }),
			c ? /* @__PURE__ */ Q("div", {
				class: "msg",
				dangerouslySetInnerHTML: { __html: c.html }
			}) : /* @__PURE__ */ Q("div", {
				class: "msg",
				dir: "auto",
				children: t.body_text
			}),
			c && c.sources.length > 0 && /* @__PURE__ */ Q("div", {
				class: "sources",
				"aria-label": H("sources"),
				children: c.sources.map((e, t) => /* @__PURE__ */ Q("a", {
					class: "src",
					href: e.href,
					target: "_blank",
					rel: "noopener noreferrer",
					style: { "--i": t },
					title: le(e.href),
					children: [/* @__PURE__ */ Q(dn, {}), /* @__PURE__ */ Q("span", { children: e.title })]
				}, e.href))
			}),
			t.ai_action && n && /* @__PURE__ */ Q(Nn, {
				a: t.ai_action,
				ui: s,
				onDecide: (r) => void e.decide(n, t.ai_action, r)
			}),
			i && /* @__PURE__ */ Q("div", {
				class: "offer",
				children: [/* @__PURE__ */ Q("button", {
					class: "handoff",
					disabled: i === "pending",
					"aria-busy": i === "pending",
					onClick: () => n && void e.handoff(n),
					children: [i === "pending" ? /* @__PURE__ */ Q(bn, {}) : fn(), H("talkToPerson")]
				}), i === "failed" && /* @__PURE__ */ Q("div", {
					class: "meta failed",
					children: H("handoffFailed")
				})]
			}),
			/* @__PURE__ */ Q("div", {
				class: "meta-row",
				children: [/* @__PURE__ */ Q("span", {
					class: "meta",
					children: Ie(t.created_at)
				}), n && !t.local && /* @__PURE__ */ Q("span", {
					class: `rate${o ? " rated" : ""}`,
					children: [/* @__PURE__ */ Q("button", {
						class: `rate-btn${o === "up" ? " on" : ""}`,
						"aria-pressed": o === "up",
						"aria-label": H("rateUp"),
						title: H("rateUp"),
						onClick: () => void e.rate(n, t, "up"),
						children: /* @__PURE__ */ Q(_n, {})
					}), /* @__PURE__ */ Q("button", {
						class: `rate-btn${o === "down" ? " on" : ""}`,
						"aria-pressed": o === "down",
						"aria-label": H("rateDown"),
						title: H("rateDown"),
						onClick: () => void e.rate(n, t, "down"),
						children: /* @__PURE__ */ Q(vn, {})
					})]
				})]
			})
		]
	});
}
var Mn = {
	confirmed: {
		label: () => H("actionConfirmed"),
		icon: () => pn(11)
	},
	declined: {
		label: () => H("actionDeclined"),
		icon: () => mn(10)
	},
	expired: {
		label: () => H("actionExpired"),
		icon: () => hn(11)
	},
	failed: {
		label: () => H("actionFailed"),
		icon: () => mn(10)
	}
};
function Nn({ a: e, ui: t, onDecide: n }) {
	let r = Oe(e, t), i = r === "pending" ? t == null ? void 0 : t.busy : void 0;
	return /* @__PURE__ */ Q("div", {
		class: `action s-${r}`,
		role: "group",
		"aria-label": e.title,
		children: [
			/* @__PURE__ */ Q("div", {
				class: "action-top",
				children: [/* @__PURE__ */ Q("span", {
					class: "action-tool",
					children: [gn(11), e.tool]
				}), r === "pending" ? /* @__PURE__ */ Q("span", {
					class: "action-need",
					children: H("actionPending")
				}) : /* @__PURE__ */ Q("span", {
					class: `pill p-${r}`,
					role: "status",
					children: [Mn[r].icon(), Mn[r].label()]
				})]
			}),
			/* @__PURE__ */ Q("div", {
				class: "action-title",
				children: e.title
			}),
			e.summary && /* @__PURE__ */ Q("div", {
				class: "action-sum",
				children: e.summary
			}),
			/* @__PURE__ */ Q("div", {
				class: "action-btns",
				"data-open": r === "pending" ? "" : void 0,
				children: /* @__PURE__ */ Q("div", { children: [/* @__PURE__ */ Q("button", {
					class: "btn ghost",
					disabled: !!i || r !== "pending",
					"aria-busy": i === "decline",
					onClick: () => n("decline"),
					children: [i === "decline" && /* @__PURE__ */ Q(bn, {}), H("cancel")]
				}), /* @__PURE__ */ Q("button", {
					class: "btn solid",
					disabled: !!i || r !== "pending",
					"aria-busy": i === "confirm",
					onClick: () => n("confirm"),
					children: [i === "confirm" && /* @__PURE__ */ Q(bn, {}), H("confirm")]
				})] })
			}),
			(t == null ? void 0 : t.error) && r === "pending" && /* @__PURE__ */ Q("div", {
				class: "err",
				children: H("actionError")
			})
		]
	});
}
function Pn(e, t) {
	let [n, r] = J(() => t ? 0 : e.length), i = X(e.length);
	return i.current = e.length, Y(() => {
		if (Cn()) {
			r(e.length);
			return;
		}
		if (n >= e.length) return;
		let t = requestAnimationFrame(() => r((e) => Math.min(i.current, e + Math.max(2, Math.ceil((i.current - e) * .18)))));
		return () => cancelAnimationFrame(t);
	}, [n, e]), e.slice(0, Math.min(n, e.length));
}
function Fn({ text: e, writing: t }) {
	let n = Pn(e, !0), r = Et(() => tt(n, !0), [n]);
	return /* @__PURE__ */ Q("div", {
		class: `md${t || n.length < e.length ? " writing" : ""}`,
		children: r
	});
}
function In({ name: e, status: t }) {
	return /* @__PURE__ */ Q("div", {
		class: `tool t-${t}`,
		children: [/* @__PURE__ */ Q("span", {
			class: "tool-ic",
			children: t === "running" ? /* @__PURE__ */ Q(bn, {}) : t === "done" ? pn(11) : mn(10)
		}), /* @__PURE__ */ Q("span", { children: H(t === "running" ? "checking" : t === "done" ? "checked" : "toolFailed", { tool: e }) })]
	});
}
function Ln({ r: e, bot: t }) {
	let n = !e.partial && e.text.trim() !== "", r = e.bot || t.name;
	return /* @__PURE__ */ Q("div", {
		class: `row them gap ai live${e.phase === "leaving" ? " leaving" : ""}`,
		"aria-busy": "true",
		children: [/* @__PURE__ */ Q(On, { bot: {
			...t,
			name: r
		} }), /* @__PURE__ */ Q("div", {
			class: "msg",
			children: [e.tools.length > 0 && /* @__PURE__ */ Q("div", {
				class: "tools",
				children: e.tools.map((e, t) => /* @__PURE__ */ Q(In, {
					name: e.name,
					status: e.status
				}, t))
			}), n ? /* @__PURE__ */ Q(Fn, {
				text: e.text,
				writing: e.phase === "streaming"
			}) : !e.tools.some((e) => e.status === "running") && /* @__PURE__ */ Q("span", {
				class: "typing-dots",
				role: "status",
				"aria-label": H("botTyping", { name: r }),
				children: /* @__PURE__ */ Q(wn, {})
			})]
		})]
	});
}
function Rn() {
	return /* @__PURE__ */ Q("div", {
		class: "handed",
		role: "status",
		children: [/* @__PURE__ */ Q("span", {
			class: "handed-ic",
			children: fn(15)
		}), /* @__PURE__ */ Q("span", {
			class: "handed-text",
			children: [/* @__PURE__ */ Q("b", { children: H("handedOff") }), /* @__PURE__ */ Q("span", { children: H("handedOffHint") })]
		})]
	});
}
//#endregion
//#region widget/src/ui.tsx
var zn = 15e3, Bn = (e) => ({
	id: "typing",
	ticket_id: e,
	bot: null,
	text: "",
	tools: [],
	phase: "typing",
	partial: !1,
	message_id: null,
	handoff: !1,
	action_id: null,
	at: 0
}), Vn = (e) => {
	var t;
	return (t = e == null ? void 0 : e.bot) == null ? {
		name: ue,
		avatar_url: null
	} : t;
}, Hn = (e) => (e == null ? void 0 : e.title) || (e == null ? void 0 : e.workspace_name) || H("support"), Un = (e) => (e == null ? void 0 : e.subtitle) || (e != null && e.ai_answers ? H("replyTimeAi", { name: Vn(e).name }) : H("replyTime"));
function Wn(e) {
	var t, n;
	let r = (t = (n = e.config) == null ? void 0 : n.theme) == null ? "auto" : t;
	return r === "auto" ? e.dark === null ? "auto" : e.dark ? "dark" : "light" : r;
}
function Gn({ c: e }) {
	var t;
	let n = Lt(e.store), r = X(null), i = X(null), [a, o] = J(""), [s, c] = J([]), [l, u] = J(!1), [d, f] = J(n.open), [p, m] = J(!1), h = n.config;
	Y(() => {
		e.announce = (e) => {
			o(""), requestAnimationFrame(() => o(e));
		};
	}, [e]), Y(() => {
		if (n.open) {
			f(!0), m(!1);
			return;
		}
		if (!d) return;
		if (Cn()) {
			f(!1);
			return;
		}
		m(!0);
		let e = setTimeout(() => {
			f(!1), m(!1);
		}, 160);
		return () => clearTimeout(e);
	}, [n.open]);
	let g = X(n.open);
	Y(() => {
		if (!n.preview) {
			var e;
			n.open && !g.current ? requestAnimationFrame(() => {
				var e, t;
				let n = r.current, i = (e = (t = n == null ? void 0 : n.querySelector("textarea:not(:disabled)")) == null ? n == null ? void 0 : n.querySelector(".primary:not(:disabled)") : t) == null ? n == null ? void 0 : n.querySelector("button") : e;
				i == null || i.focus({ preventScroll: !0 });
			}) : !n.open && g.current && ((e = i.current) == null || e.focus());
		}
		g.current = n.open;
	}, [n.open, n.preview]);
	let _ = (t) => {
		if (t.key === "Escape") {
			t.stopPropagation(), e.close();
			return;
		}
		if (t.key !== "Tab" || !r.current) return;
		let n = Array.from(r.current.querySelectorAll("button:not(:disabled), textarea:not(:disabled), input:not([hidden]), a[href]")).filter((e) => e.offsetParent !== null);
		if (!n.length) return;
		let i = r.current.getRootNode(), a = n[0], o = n[n.length - 1];
		t.shiftKey && i.activeElement === a ? (t.preventDefault(), o.focus()) : !t.shiftKey && i.activeElement === o && (t.preventDefault(), a.focus());
	}, v = (t) => {
		if (!t) return;
		let r = Array.from(t).slice(0, 5);
		r.length && (n.view !== "thread" && e.newConversation(), c((e) => [...e, ...r].slice(0, 5)));
	}, y = (h == null ? void 0 : h.launcher) === "label";
	return /* @__PURE__ */ Q("div", {
		class: [
			"root",
			(h == null ? void 0 : h.position) === "left" ? "pos-left" : "pos-right",
			`radius-${(t = h == null ? void 0 : h.radius) == null ? "soft" : t}`,
			n.preview ? "preview" : "",
			h ? "" : "pending"
		].filter(Boolean).join(" "),
		"data-theme": Wn(n),
		dir: Ne(n.lang) ? "rtl" : "ltr",
		lang: Me(),
		onPointerDown: () => (h == null ? void 0 : h.sound) && at(),
		children: [
			/* @__PURE__ */ Q("div", {
				class: "sr",
				"aria-live": "polite",
				children: a
			}),
			(n.open || d) && /* @__PURE__ */ Q("div", {
				ref: r,
				class: `panel${!n.open && p ? " leaving" : ""}`,
				role: "dialog",
				"aria-hidden": !n.open || void 0,
				"aria-label": H("chatWith", { name: Hn(h) }),
				onKeyDown: _,
				onDragOver: (e) => {
					var t;
					(t = e.dataTransfer) != null && t.types.includes("Files") && (e.preventDefault(), u(!0));
				},
				onDragLeave: (e) => {
					e.target === r.current && u(!1);
				},
				onDrop: (e) => {
					var t, n;
					e.preventDefault(), u(!1), v((t = (n = e.dataTransfer) == null ? void 0 : n.files) == null ? null : t);
				},
				children: [n.view === "home" ? /* @__PURE__ */ Q(Yn, { c: e }) : /* @__PURE__ */ Q(Xn, {
					c: e,
					files: s,
					setFiles: c,
					addFiles: v
				}), l && /* @__PURE__ */ Q("div", {
					class: "drop",
					children: H("dropFiles")
				})]
			}),
			h && /* @__PURE__ */ Q("button", {
				ref: i,
				class: `launcher${y ? " pill" : ""}${n.open ? " hide-mobile" : ""}`,
				"aria-label": n.open ? H("closeChat") : n.unread ? H("openChatUnread", { count: n.unread }) : H("openChat"),
				"aria-expanded": n.open,
				onClick: () => e.toggle(),
				children: [
					/* @__PURE__ */ Q("span", {
						class: "l-ic",
						children: [/* @__PURE__ */ Q("span", {
							class: "ic ic-open",
							children: h.launcher_icon_url ? /* @__PURE__ */ Q("img", {
								src: h.launcher_icon_url,
								alt: "",
								referrerpolicy: "no-referrer"
							}) : yn(y ? 20 : 24)
						}), /* @__PURE__ */ Q("span", {
							class: "ic ic-x",
							children: nn(y ? 18 : 22)
						})]
					}),
					y && /* @__PURE__ */ Q("span", {
						class: "l-label",
						"aria-hidden": "true",
						children: /* @__PURE__ */ Q("span", { children: h.launcher_label || H("chatWithUs") })
					}),
					!n.open && n.unread > 0 && /* @__PURE__ */ Q("span", {
						class: "badge",
						children: n.unread > 9 ? "9+" : n.unread
					}, n.unread)
				]
			})
		]
	});
}
function Kn(e) {
	let t = e == null ? void 0 : e.trim();
	return !t || qn.has(t.toLowerCase()) ? H("greeting") : t;
}
var qn = /* @__PURE__ */ new Set(["hi there. how can we help?", "hi there! how can we help?"]);
function Jn({ cfg: e }) {
	let t = Hn(e);
	return /* @__PURE__ */ Q("span", {
		class: "ws-glyph",
		"aria-hidden": "true",
		children: e != null && e.avatar_url ? /* @__PURE__ */ Q("img", {
			src: e.avatar_url,
			alt: "",
			referrerpolicy: "no-referrer"
		}) : t.charAt(0).toUpperCase()
	});
}
function Yn({ c: e }) {
	var t, n;
	let r = Lt(e.store), i = r.config, a = r.conversations, o = (t = i == null ? void 0 : i.team) == null ? [] : t, s = i != null && i.ai_answers ? Vn(i) : null, c = !!r.session, l = (n = i == null ? void 0 : i.suggestions) == null ? [] : n;
	return /* @__PURE__ */ Q(y, { children: [
		/* @__PURE__ */ Q("div", {
			class: "head home",
			children: [
				/* @__PURE__ */ Q("div", {
					class: "head-row",
					children: [/* @__PURE__ */ Q("div", {
						class: "ws",
						children: [/* @__PURE__ */ Q(Jn, { cfg: i }), /* @__PURE__ */ Q("span", {
							class: "ws-name",
							children: Hn(i)
						})]
					}), /* @__PURE__ */ Q("button", {
						class: "icon-btn",
						"aria-label": H("closeChat"),
						onClick: () => e.close(),
						children: nn()
					})]
				}),
				/* @__PURE__ */ Q("div", {
					class: "greet",
					dir: "auto",
					children: Kn(i == null ? void 0 : i.greeting)
				}),
				/* @__PURE__ */ Q("div", {
					class: "sub-row",
					children: [/* @__PURE__ */ Q(Dn, {
						team: o,
						bot: s,
						size: 22
					}), /* @__PURE__ */ Q("span", {
						class: "sub",
						children: Un(i)
					})]
				})
			]
		}),
		/* @__PURE__ */ Q("div", {
			class: "body",
			children: /* @__PURE__ */ Q("div", {
				class: "home-body",
				children: [
					r.error && !r.session ? /* @__PURE__ */ Q("div", {
						class: "empty",
						children: [/* @__PURE__ */ Q("b", { children: H("unavailable") }), H("unavailableHint")]
					}) : /* @__PURE__ */ Q("button", {
						class: "primary start",
						disabled: !c,
						onClick: () => e.newConversation(),
						children: [/* @__PURE__ */ Q("span", { children: H("sendMessage") }), /* @__PURE__ */ Q("span", {
							class: "start-ic flip",
							children: /* @__PURE__ */ Q(ln, {})
						})]
					}),
					l.length > 0 && /* @__PURE__ */ Q("div", {
						class: "suggest",
						role: "group",
						"aria-label": H("suggested"),
						children: l.map((t, n) => /* @__PURE__ */ Q("button", {
							class: "sugg",
							style: { "--i": n },
							disabled: !c,
							onClick: () => void e.startWith(t),
							dir: "auto",
							children: t
						}, t))
					}),
					a === null ? r.booting && /* @__PURE__ */ Q("div", {
						class: "card",
						"aria-busy": "true",
						children: [/* @__PURE__ */ Q("div", {
							class: "skel",
							style: { width: "40%" }
						}), /* @__PURE__ */ Q("div", {
							class: "skel",
							style: { width: "75%" }
						})]
					}) : a.length > 0 ? /* @__PURE__ */ Q("div", {
						class: "card",
						children: [/* @__PURE__ */ Q("div", {
							class: "card-h",
							children: H("yourConversations")
						}), a.slice(0, 6).map((t) => /* @__PURE__ */ Q("button", {
							class: "conv",
							onClick: () => void e.openConversation(t.id),
							children: [/* @__PURE__ */ Q("span", {
								class: `dot${t.status === "solved" || t.status === "closed" ? "" : " open"}`,
								"aria-hidden": "true"
							}), /* @__PURE__ */ Q("span", {
								class: "conv-main",
								children: [/* @__PURE__ */ Q("span", {
									class: "conv-top",
									children: [/* @__PURE__ */ Q("span", {
										class: "conv-subject",
										children: t.subject || H("conversation")
									}), /* @__PURE__ */ Q("span", {
										class: "time",
										children: Fe(t.last_message_at)
									})]
								}), /* @__PURE__ */ Q("span", {
									class: "conv-prev",
									children: r.typing[t.id] ? H("typing", { name: r.typing[t.id] }) : t.last_message_preview
								})]
							})]
						}, t.id))]
					}) : null,
					(i == null ? void 0 : i.help_center_url) && /* @__PURE__ */ Q("div", {
						class: "card",
						children: /* @__PURE__ */ Q("a", {
							class: "link-row",
							href: i.help_center_url,
							target: "_blank",
							rel: "noopener noreferrer",
							children: [H("searchHelp"), /* @__PURE__ */ Q("span", {
								class: "flip",
								children: /* @__PURE__ */ Q(cn, {})
							})]
						})
					})
				]
			})
		}),
		/* @__PURE__ */ Q("a", {
			class: "foot",
			href: "https://opendesk.sh",
			target: "_blank",
			rel: "noopener noreferrer",
			children: [/* @__PURE__ */ Q(xn, {}), H("poweredBy")]
		})
	] });
}
function Xn({ c: e, files: t, setFiles: n, addFiles: r }) {
	var i, a, o, s, c, l;
	let u = Lt(e.store), d = u.config, f = Vn(d), p = (i = u.active) == null ? Rt : i, m = u.messages[p], h = (a = u.conversations) == null ? void 0 : a.find((e) => e.id === u.active), g = u.active ? u.typing[u.active] : void 0, _ = e.needsEmail(), v = !!u.active && ((h == null ? void 0 : h.ai_handoff) === !0 || u.handoff[u.active] === "done"), b = Ee({
		aiAnswers: (d == null ? void 0 : d.ai_answers) === !0,
		conversation: h,
		handedOff: v,
		msgs: m == null ? [] : m
	}), x = u.active ? He(u.live, u.active) : [], S = x.some((e) => e.phase !== "leaving"), C = u.active ? u.awaiting[u.active] : void 0, [w, T] = J(Date.now()), E = m == null ? void 0 : m[m.length - 1], D = !!(E != null && E.ai_action) && (E.ai_action.status !== "pending" || !!((o = u.actions[E.ai_action.id]) != null && o.status)), O = b && !!E && C !== void 0 && w - C < zn && !g && !x.length && (E.direction === "inbound" || D);
	Y(() => {
		if (C === void 0) return;
		T(Date.now());
		let e = C + zn - Date.now();
		if (e <= 0) return;
		let t = setTimeout(() => T(Date.now()), e + 20);
		return () => clearTimeout(t);
	}, [C]);
	let k = m ? De(m) : null, A = [];
	(m == null ? [] : m).forEach((t, n) => {
		var r, i, a;
		let o = m[n - 1], s = t.direction === "inbound", c = Ce(t), l = !o || new Date(o.created_at).toDateString() !== new Date(t.created_at).toDateString(), d = !!o && !l && !c && !Ce(o) && o.direction === t.direction && o.author.type === t.author.type && ((r = o.author.user) == null ? void 0 : r.id) === ((i = t.author.user) == null ? void 0 : i.id) && Date.parse(t.created_at) - Date.parse(o.created_at) < 3e5, h = c && u.active && t.id === (k == null ? void 0 : k.id) && b && !S && n === m.length - 1 ? u.handoff[u.active] === "failed" ? "failed" : u.handoff[u.active] === "pending" ? "pending" : "idle" : null;
		A.push(/* @__PURE__ */ Q(y, { children: [l ? /* @__PURE__ */ Q("div", {
			class: "day",
			children: Le(t.created_at)
		}) : null, c ? /* @__PURE__ */ Q(jn, {
			c: e,
			m: t,
			convId: u.active,
			bot: f,
			offer: h,
			swapped: !!u.swapped[t.id],
			rating: t.id in u.ratings ? u.ratings[t.id] : (a = t.ai_rating) == null ? null : a,
			action: t.ai_action ? u.actions[t.ai_action.id] : void 0
		}) : /* @__PURE__ */ Q(kn, {
			m: t,
			me: s,
			grouped: d,
			onRetry: () => void e.retry(p, t.id)
		})] }, t.id));
	}), x.forEach((e, t) => A.push(/* @__PURE__ */ Q(Ln, {
		r: e,
		bot: f
	}, t === 0 ? "ai-live" : `live-${e.id}`))), O && A.push(/* @__PURE__ */ Q(Ln, {
		r: Bn((s = u.active) == null ? Rt : s),
		bot: f
	}, "ai-live"));
	let j = b ? {
		bot: f,
		team: []
	} : {
		bot: null,
		team: (c = d == null ? void 0 : d.team) == null ? [] : c
	};
	return /* @__PURE__ */ Q(y, { children: [
		/* @__PURE__ */ Q("div", {
			class: "head",
			children: [
				/* @__PURE__ */ Q("button", {
					class: "icon-btn flip",
					"aria-label": H("back"),
					onClick: () => e.home(),
					children: /* @__PURE__ */ Q(rn, {})
				}),
				/* @__PURE__ */ Q(Dn, {
					team: j.team,
					bot: j.bot,
					size: 26
				}),
				/* @__PURE__ */ Q("div", {
					class: "head-main",
					children: [/* @__PURE__ */ Q("div", {
						class: "title",
						children: (h == null ? void 0 : h.subject) || (u.active ? H("conversation") : Hn(d))
					}), /* @__PURE__ */ Q("div", {
						class: "title-sub",
						children: b ? Un(d) : h != null && h.subject || u.active ? Hn(d) : Un(d)
					})]
				}),
				/* @__PURE__ */ Q("button", {
					class: "icon-btn",
					"aria-label": H("closeChat"),
					onClick: () => e.close(),
					children: nn()
				})
			]
		}),
		/* @__PURE__ */ Q(Zn, {
			watch: `${p}:${(l = m == null ? void 0 : m.length) == null ? -1 : l}:${A.length}`,
			children: [
				_ && /* @__PURE__ */ Q(Qn, { c: e }),
				m === void 0 ? /* @__PURE__ */ Q("div", {
					"aria-busy": "true",
					style: { paddingTop: "12px" },
					children: [/* @__PURE__ */ Q("div", {
						class: "skel",
						style: { width: "55%" }
					}), /* @__PURE__ */ Q("div", {
						class: "skel",
						style: {
							width: "35%",
							marginInlineStart: "auto"
						}
					})]
				}) : A.length === 0 && !_ ? /* @__PURE__ */ Q("div", {
					class: "empty",
					children: [/* @__PURE__ */ Q("b", {
						dir: "auto",
						children: Kn(d == null ? void 0 : d.greeting)
					}), H("emptyHint")]
				}) : /* @__PURE__ */ Q("div", {
					class: "thread",
					children: A
				}),
				v && m && !we(m) && /* @__PURE__ */ Q(Rn, {}),
				g && /* @__PURE__ */ Q("div", {
					class: "typing",
					role: "status",
					"aria-label": H("typing", { name: g }),
					children: [/* @__PURE__ */ Q(wn, {}), /* @__PURE__ */ Q("span", {
						"aria-hidden": "true",
						children: g
					})]
				})
			]
		}),
		/* @__PURE__ */ Q(er, {
			c: e,
			files: t,
			setFiles: n,
			addFiles: r,
			disabled: _ || !u.session
		})
	] });
}
function Zn({ watch: e, children: t }) {
	let n = X(null), r = X(null), i = X(!0), [a, o] = J(!1), s = X(!0), c = (e) => {
		let t = n.current;
		t && (e && !Cn() && typeof t.scrollTo == "function" ? t.scrollTo({
			top: t.scrollHeight,
			behavior: "smooth"
		}) : t.scrollTop = t.scrollHeight);
	};
	return Tt(() => {
		s.current || i.current ? c(!s.current) : o(!0), s.current = !1;
	}, [e]), Y(() => {
		let e = r.current;
		if (!e || typeof ResizeObserver > "u") return;
		let t = new ResizeObserver(() => {
			i.current && c(!1);
		});
		return t.observe(e), () => t.disconnect();
	}, []), /* @__PURE__ */ Q("div", {
		class: "body",
		ref: n,
		onScroll: () => {
			let e = n.current;
			e && (i.current = e.scrollHeight - e.scrollTop - e.clientHeight < 48, i.current && a && o(!1));
		},
		children: [/* @__PURE__ */ Q("div", {
			ref: r,
			class: "body-inner",
			children: t
		}), a && /* @__PURE__ */ Q("button", {
			class: "jump",
			"aria-label": H("jumpLatest"),
			onClick: () => {
				i.current = !0, o(!1), c(!0);
			},
			children: /* @__PURE__ */ Q(sn, {})
		})]
	});
}
function Qn({ c: e }) {
	let [t, n] = J(""), [r, i] = J(""), [a, o] = J(null), [s, c] = J(!1);
	return /* @__PURE__ */ Q("form", {
		class: "capture",
		onSubmit: async (n) => {
			if (n.preventDefault(), !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(r.trim())) return o(H("invalidEmail"));
			c(!0), o(null);
			try {
				await e.captureEmail(t.trim(), r.trim());
			} catch {
				o(H("saveFailed"));
			} finally {
				c(!1);
			}
		},
		children: [
			/* @__PURE__ */ Q("p", { children: H("captureHint") }),
			/* @__PURE__ */ Q("input", {
				class: "field",
				placeholder: H("name"),
				"aria-label": H("name"),
				value: t,
				onInput: (e) => n(e.currentTarget.value),
				autocomplete: "name"
			}),
			/* @__PURE__ */ Q("input", {
				class: "field",
				type: "email",
				placeholder: H("emailPlaceholder"),
				"aria-label": H("email"),
				dir: "ltr",
				value: r,
				onInput: (e) => i(e.currentTarget.value),
				autocomplete: "email",
				required: !0
			}),
			a && /* @__PURE__ */ Q("div", {
				class: "err",
				children: a
			}),
			/* @__PURE__ */ Q("button", {
				class: "primary",
				type: "submit",
				disabled: s,
				style: { height: "36px" },
				children: H(s ? "saving" : "continue")
			})
		]
	});
}
var $n = 120;
function er({ c: e, files: t, setFiles: n, addFiles: r, disabled: i }) {
	let [a, o] = J(""), [s, c] = J(30), l = X(null), u = X(null), d = X(null), f = !i && (!!a.trim() || t.length > 0);
	Tt(() => {
		let e = u.current;
		e && (e.value = a, c(Math.max(30, Math.min(e.scrollHeight, $n))));
	}, [a]);
	let p = () => {
		var r;
		f && (e.send(a, t), o(""), n([]), (r = l.current) == null || r.focus());
	};
	return /* @__PURE__ */ Q("div", {
		class: "composer",
		children: [t.length > 0 && /* @__PURE__ */ Q("div", {
			class: "pending-files",
			children: t.map((e, r) => /* @__PURE__ */ Q("span", {
				class: "chip",
				children: [
					/* @__PURE__ */ Q(un, {}),
					/* @__PURE__ */ Q("span", { children: e.name }),
					/* @__PURE__ */ Q("button", {
						"aria-label": H("removeFile", { name: e.name }),
						onClick: () => n(t.filter((e, t) => t !== r)),
						children: nn(12)
					})
				]
			}, `${e.name}-${r}`))
		}), /* @__PURE__ */ Q("div", {
			class: `cbar${i ? " off" : ""}`,
			children: [
				/* @__PURE__ */ Q("div", {
					class: "input-wrap",
					children: [/* @__PURE__ */ Q("textarea", {
						ref: l,
						class: "input",
						rows: 1,
						value: a,
						style: {
							height: `${s}px`,
							overflowY: s >= $n ? "auto" : "hidden"
						},
						disabled: i,
						placeholder: H(i ? "placeholderEmail" : "placeholder"),
						"aria-label": H("message"),
						dir: "auto",
						onInput: (t) => {
							o(t.currentTarget.value), e.typing();
						},
						onKeyDown: (e) => {
							e.key === "Enter" && !e.shiftKey && !e.isComposing && (e.preventDefault(), p());
						},
						onPaste: (e) => {
							var t;
							let n = (t = e.clipboardData) == null ? void 0 : t.files;
							n && n.length && (e.preventDefault(), r(n));
						}
					}), /* @__PURE__ */ Q("textarea", {
						ref: u,
						class: "input mirror",
						rows: 1,
						tabIndex: -1,
						"aria-hidden": "true",
						disabled: !0
					})]
				}),
				/* @__PURE__ */ Q("input", {
					ref: d,
					type: "file",
					multiple: !0,
					hidden: !0,
					onChange: (e) => {
						r(e.currentTarget.files), e.currentTarget.value = "";
					}
				}),
				/* @__PURE__ */ Q("button", {
					class: "icon-btn",
					"aria-label": H("attach"),
					disabled: i,
					onClick: () => {
						var e;
						return (e = d.current) == null ? void 0 : e.click();
					},
					children: /* @__PURE__ */ Q(an, {})
				}),
				/* @__PURE__ */ Q("button", {
					class: `send${f ? " ready" : ""}`,
					"aria-label": H("send"),
					disabled: !f,
					onClick: p,
					children: /* @__PURE__ */ Q(on, {})
				})
			]
		})]
	});
}
//#endregion
//#region widget/src/preview.tsx
function tr(e, t) {
	var n;
	let r = { ...t }, i = !0, a = document.createElement("div");
	a.setAttribute("data-opendesk-preview", ""), a.style.cssText = "position:absolute;inset:0;overflow:hidden;transform:translateZ(0);contain:layout paint;border-radius:inherit;", getComputedStyle(e).position === "static" && (e.style.position = "relative"), e.appendChild(a);
	let o = a.attachShadow({ mode: "open" }), s = document.createElement("style");
	s.textContent = en, o.appendChild(s);
	let c = document.createElement("div");
	o.appendChild(c);
	let l = Zt({
		seed: t.autoplay ? "empty" : "preview",
		latency: 60
	}), u = new It({
		preview: !0,
		dark: (n = t.dark) == null ? null : n
	}), d = new Wt({
		base: "https://preview.invalid",
		key: "wk_preview",
		fetcher: l.fetch,
		socketFactory: l.socket,
		persist: !1,
		store: u
	}), f = (e) => {
		d.setConfig(e);
		let t = u.get().config;
		t && ve(a, t);
	}, p = !1, m = (e) => {
		e === "launcher" ? u.set({ open: !1 }) : e === "home" ? u.set({
			open: !0,
			view: "home",
			active: null
		}) : (u.set({
			open: !0,
			view: "thread",
			active: l.showcaseId
		}), p && d.openConversation(l.showcaseId).then(() => i && l.playShowcase()));
	}, h = t.autoplay ? rr(e, a, o, d, u, () => i) : null;
	return f(r.config), h || m(r.scene), I(/* @__PURE__ */ Q(Gn, { c: d }), c), d.boot(r.config).then(() => {
		i && (p = !0, f(r.config), h ? h.start() : m(r.scene));
	}), {
		update(e) {
			var t;
			if (!i) return;
			let n = r;
			r = {
				...r,
				...Object.fromEntries(Object.entries(e).filter(([, e]) => e !== void 0))
			}, e.config && e.config !== n.config && f(e.config), "dark" in e && u.set({ dark: (t = e.dark) == null ? null : t }), e.scene && e.scene !== n.scene && m(e.scene);
		},
		destroy() {
			i && (i = !1, h == null || h.stop(), I(null, c), d.stop(), l.dispose(), a.remove());
		}
	};
}
var nr = {
	pick: "Change my delivery address",
	order: "It's order #4821, please send it to 22 Baker Street, London."
};
function rr(e, t, n, r, i, a) {
	let o = 0, s = !1, c = !1, l, u = /* @__PURE__ */ new Set(), d = new IntersectionObserver(([e]) => {
		s = e.isIntersecting, s && u.forEach((e) => e());
	});
	d.observe(e);
	class f extends Error {}
	class p extends Error {}
	let m = (e) => {
		if (e !== o || !a()) throw new f();
	}, h = async (e, t) => {
		m(e), s || await new Promise((e) => u.add(function t() {
			u.delete(t), e();
		})), await new Promise((e) => setTimeout(e, t)), m(e);
	}, g = (e, t, n = 15e3) => new Promise((r, s) => {
		let c = () => {
			if (e !== o || !a()) return l(new f());
			t() && l();
		}, l = (e) => {
			u(), clearInterval(d), clearTimeout(m), e ? s(e) : r();
		}, u = i.subscribe(c), d = setInterval(c, 120), m = setTimeout(() => l(new p()), n);
		c();
	}), _ = (e) => n.querySelector(e), v = () => {
		let e = i.get();
		return Object.keys(e.live.replies).length === 0 && Object.keys(e.awaiting).length === 0;
	};
	async function y(e) {
		var t;
		await h(e, 900), r.open(), await g(e, () => !!_(".sugg:not(:disabled)")), await h(e, 1700);
		let i = [...n.querySelectorAll(".sugg")].find((e) => {
			var t;
			return ((t = e.textContent) == null ? void 0 : t.trim()) === nr.pick;
		});
		i == null || i.click(), await h(e, 400), await g(e, v), await h(e, 900);
		let a = _("textarea.input:not(.mirror)");
		if (!a) throw new f();
		for (let t = 1; t <= nr.order.length; t++) a.value = nr.order.slice(0, t), a.dispatchEvent(new Event("input", { bubbles: !0 })), await h(e, 26 + t * 17 % 40);
		await h(e, 350), a.value = "", a.dispatchEvent(new Event("input", { bubbles: !0 })), r.send(nr.order, []), await g(e, () => !!_(".btn.solid:not(:disabled)"), 2e4), await h(e, 2200), (t = _(".btn.solid:not(:disabled)")) == null || t.click(), await h(e, 400), await g(e, v), await h(e, 4200), r.close(), await h(e, 600), r.home(), await h(e, 2200);
	}
	async function b() {
		let e = ++o;
		for (;;) try {
			await y(e);
		} catch (e) {
			if (e instanceof f) return;
			if (!(e instanceof p)) throw e;
			r.close(), r.home();
		}
	}
	let x = (e) => {
		e.isTrusted && e.composedPath()[0] !== t && (c || (c = !0, o++), S());
	}, S = () => {
		clearTimeout(l), l = setTimeout(function e() {
			a() && (i.get().open ? l = setTimeout(e, 8e3) : (c = !1, r.home(), b()));
		}, 9e3);
	};
	return t.addEventListener("pointerdown", x, !0), t.addEventListener("keydown", x, !0), {
		start() {
			var e, t;
			(e = (t = window).matchMedia) != null && e.call(t, "(prefers-reduced-motion: reduce)").matches ? r.open() : b();
		},
		stop() {
			o++, clearTimeout(l), d.disconnect(), u.clear();
		}
	};
}
//#endregion
export { Xt as demoConfig, tr as mountPreview };
