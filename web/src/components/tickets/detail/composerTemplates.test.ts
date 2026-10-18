import { describe, expect, it } from "vitest";
import { signatureText, variableText } from "./composerTemplates";

describe("variableText", () => {
  it("writes the ticket's value as plain text", () => {
    expect(variableText("contact.first_name", "Ada")).toBe("Ada");
  });
  it("writes the tag when there is no value yet, so the server fills it on send", () => {
    expect(variableText("contact.first_name", undefined)).toBe("{{ contact.first_name }}");
    expect(variableText("contact.first_name", "")).toBe("{{ contact.first_name }}");
    expect(variableText("contact.first_name", "  ")).toBe("{{ contact.first_name }}");
  });
  it("writes the tag for HTML values", () => {
    expect(variableText("agent.signature", "<p>Ada</p>", true)).toBe("{{ agent.signature }}");
  });
  it("never writes a value that would be read as a template on send", () => {
    expect(variableText("contact.name", "{{ agent.phone }}")).toBe("{{ contact.name }}");
    expect(variableText("contact.name", "{% if x %}")).toBe("{{ contact.name }}");
  });
});

describe("signatureText", () => {
  it("reads a signature as one line", () => {
    expect(signatureText("<p>Ada Lovelace<br>Support &amp; success</p><p>Acme</p>")).toBe("Ada Lovelace Support & success Acme");
  });
  it("is null when empty or too long", () => {
    expect(signatureText("")).toBeNull();
    expect(signatureText("<p> </p>")).toBeNull();
    expect(signatureText(`<p>${"a".repeat(121)}</p>`)).toBeNull();
    expect(signatureText(`<p>${"a".repeat(120)}</p>`)).toBe("a".repeat(120));
  });
  it("leaves out styles and scripts", () => {
    expect(signatureText("<style>p{color:red}</style><table><tr><td>Ada</td><td>Acme</td></tr></table><script>x()</script>")).toBe("Ada Acme");
  });
});
