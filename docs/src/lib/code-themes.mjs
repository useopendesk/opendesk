// Monochrome code themes from the brand palette: shape through weight and shade, not hue.
const theme = (type, c) => ({
  name: `opendesk-${type}`,
  type,
  colors: {
    'editor.background': c.bg,
    'editor.foreground': c.fg,
    'editor.selectionBackground': c.sel,
    'terminal.ansiBlack': c.fg,
  },
  tokenColors: [
    { scope: ['comment', 'punctuation.definition.comment'], settings: { foreground: c.faint, fontStyle: 'italic' } },
    { scope: ['string', 'string.quoted', 'string.template', 'markup.inline.raw'], settings: { foreground: c.muted } },
    { scope: ['constant.numeric', 'constant.language', 'constant.character'], settings: { foreground: c.muted } },
    { scope: ['keyword', 'storage', 'storage.type', 'storage.modifier', 'keyword.operator.new'], settings: { foreground: c.strong, fontStyle: 'bold' } },
    { scope: ['entity.name.function', 'support.function', 'entity.name.type', 'support.type', 'entity.name.class'], settings: { foreground: c.strong } },
    { scope: ['variable.parameter', 'variable.other.property', 'meta.object-literal.key', 'support.type.property-name', 'entity.name.tag'], settings: { foreground: c.fg } },
    { scope: ['punctuation', 'keyword.operator', 'meta.brace'], settings: { foreground: c.faint } },
    { scope: ['variable.other.constant', 'variable.language'], settings: { foreground: c.strong } },
    { scope: ['markup.heading', 'markup.bold'], settings: { foreground: c.strong, fontStyle: 'bold' } },
    { scope: ['markup.inserted'], settings: { foreground: c.strong } },
    { scope: ['markup.deleted'], settings: { foreground: c.faint } },
  ],
});

export const opendeskLight = theme('light', {
  bg: '#fafafa', fg: '#262626', strong: '#0a0a0a', muted: '#525252', faint: '#8a8a8a', sel: '#e4e4e4',
});
export const opendeskDark = theme('dark', {
  bg: '#1a1b1e', fg: '#dcdcde', strong: '#f5f5f6', muted: '#a1a1a6', faint: '#74747a', sel: '#2c2d31',
});
