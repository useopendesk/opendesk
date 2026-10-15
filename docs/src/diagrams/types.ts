// The shape of a docs diagram (Diagram.astro). Coordinates are SVG user units, which
// are CSS pixels at the diagram's natural width; the picture scales down with the column.

/** Colour of a box, a group or a line. Soft tints, as the app's pills. */
export type Tone = 'plain' | 'soft' | 'ink' | 'green' | 'blue' | 'amber' | 'violet' | 'red';

/** A side of a box, where a line leaves or arrives. */
export type Side = 'l' | 'r' | 't' | 'b';

export interface Box {
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
  title: string;
  /** Smaller lines under the title; `\n` breaks a line. */
  sub?: string;
  /** Short points, left aligned under the title. */
  points?: string[];
  tone?: Tone;
  /** Title in Geist Mono, for names of services, topics and headers. */
  mono?: boolean;
  /** `db` draws a cylinder. */
  shape?: 'box' | 'db';
  /** A numbered badge on the top-left corner, as on the screenshots. */
  n?: number | string;
  /** Smaller type, for a box inside another. */
  small?: boolean;
  /** Left aligned text (default centred; boxes with points are always left aligned). */
  align?: 'left' | 'center';
  /** Text at the top, for a box with another box inside it (default middle; a box with
   * points starts at the top unless this is 'middle'). */
  valign?: 'top' | 'middle';
}

export interface Group {
  x: number;
  y: number;
  w: number;
  h: number;
  label: string;
  tone?: Tone;
  /** Dashed outline, for a boundary such as a network (default solid). */
  dashed?: boolean;
}

export interface Edge {
  from: string;
  to: string;
  fromSide?: Side;
  toSide?: Side;
  /** Where along the side, 0 to 1 (default 0.5). */
  fromAt?: number;
  toAt?: number;
  /** Corner points between the two ends, for a route the default elbow does not take. */
  via?: [number, number][];
  label?: string;
  /** Where the label sits (default: the middle of the longest segment). */
  labelAt?: [number, number];
  labelAnchor?: 'start' | 'middle' | 'end';
  /** Label in Geist Mono. */
  mono?: boolean;
  dashed?: boolean;
  /** Heading colour instead of the quiet line colour. */
  strong?: boolean;
  /** Arrowheads at both ends. */
  both?: boolean;
  /** No arrowhead at all. */
  plain?: boolean;
}

/** Free text: a remark beside the boxes. */
export interface Note {
  x: number;
  y: number;
  text: string;
  anchor?: 'start' | 'middle' | 'end';
  mono?: boolean;
  tone?: Tone;
}

export interface DiagramSpec {
  /** What the diagram shows, in one or two sentences: its alt text and its Markdown copy. */
  alt: string;
  /** The caption under it. */
  caption?: string;
  width: number;
  height: number;
  /** Narrowest it is drawn before the frame scrolls sideways (default 560). */
  minWidth?: number;
  groups?: Group[];
  boxes: Box[];
  edges?: Edge[];
  notes?: Note[];
}
