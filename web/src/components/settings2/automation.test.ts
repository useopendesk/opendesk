import { describe, expect, it } from "vitest";
import { actionLabel, actionNeedsValue, aiActions, fieldLabel, offered, opLabel, opsFor, sentimentLabel, triggerLabel } from "./automation";

describe("automation AI helpers", () => {
  it("offers AI actions only when AI is on, but keeps one in use", () => {
    const keys = ["add_tag", "ai_triage"] as const;
    expect(offered([...keys], aiActions, false)).toEqual(["add_tag"]);
    expect(offered([...keys], aiActions, true)).toEqual(["add_tag", "ai_triage"]);
    expect(offered([...keys], aiActions, false, "ai_triage")).toEqual(["add_tag", "ai_triage"]);
  });

  it("offers is / is not for sentiment and needs no value for triage", () => {
    expect(opsFor("sentiment")).toEqual(["equals", "not_equals"]);
    expect(actionNeedsValue("ai_triage")).toBe(false);
    expect(actionNeedsValue("set_field")).toBe(false);
    expect(sentimentLabel("frustrated")).toBe("Frustrated");
    expect(sentimentLabel("custom")).toBe("custom");
  });
});

describe("automation labels", () => {
  it("reads every label from the catalogs", () => {
    expect(triggerLabel.ticket_created).toBe("A ticket is created");
    expect(opLabel.not_contains).toBe("doesn't contain");
    expect(actionLabel.add_tag).toBe("Add label");
    expect(actionLabel.remove_tag).toBe("Remove label");
    expect(fieldLabel.tag).toBe("Label");
    expect(fieldLabel.from_email).toBe("Sender email");
    expect(Object.keys(fieldLabel)).toContain("sentiment");
    expect(Object.keys(actionLabel).at(-1)).toBe("ai_triage");
    expect(actionLabel.set_field).toBe("Set field");
  });
});
