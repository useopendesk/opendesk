import { describe, expect, it } from "vitest";
import { firstName, peopleOf, personName } from "./people";

const account = { email: "me@margin.app", display_name: "Me", signature_html: "", identities: [] };

describe("peopleOf", () => {
  it("leaves out your own addresses and keeps the order, each address once", () => {
    const people = peopleOf(
      [
        { name: "Dana Lee", email: "Dana@Client.test" },
        { name: "", email: "me@margin.app" },
        { name: "Sam", email: "sam@client.test" },
        { name: "Dana", email: "dana@client.test" },
        { name: "Other box", email: "box@margin.app" },
      ],
      [account, { email: "box@margin.app", display_name: "", signature_html: "" }],
    );
    expect(people).toEqual([
      { name: "Dana Lee", email: "dana@client.test" },
      { name: "Sam", email: "sam@client.test" },
    ]);
  });

  it("names someone without a name by their address", () => {
    expect(personName({ name: " ", email: "x@y.test" })).toBe("x@y.test");
    expect(firstName({ name: "Dana Lee", email: "x@y.test" })).toBe("Dana");
    expect(firstName({ name: "", email: "x@y.test" })).toBe("x@y.test");
  });
});
