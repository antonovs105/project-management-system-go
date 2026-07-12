import { describe, expect, it } from "vitest";
import { dateTime, plural, relativeDate } from "../src/lib/format";
import { messages, supportedLocales } from "../src/lib/i18n-messages";

function placeholders(value: string): string[] {
  return [...value.matchAll(/\{([a-zA-Z0-9_]+)\}/g)].map((match) => match[1]).sort();
}

describe("localization contract", () => {
  it("keeps every locale complete with matching placeholders", () => {
    const englishKeys = Object.keys(messages.en).sort();
    for (const locale of supportedLocales) {
      expect(Object.keys(messages[locale]).sort()).toEqual(englishKeys);
      for (const key of englishKeys) {
        const typedKey = key as keyof typeof messages.en;
        expect(messages[locale][typedKey].trim(), `${locale}.${key}`).not.toBe("");
        expect(placeholders(messages[locale][typedKey]), `${locale}.${key}`).toEqual(placeholders(messages.en[typedKey]));
      }
    }
  });

  it("formats relative and absolute dates with the active locale", () => {
    const now = Date.parse("2026-07-12T12:00:00Z");
    expect(relativeDate("2026-07-10T12:00:00Z", "en", now)).toContain("2 days ago");
    expect(relativeDate("2026-07-10T12:00:00Z", "uk", now)).not.toContain("days ago");
    expect(dateTime("2026-07-12T12:00:00Z", "uk")).not.toEqual(dateTime("2026-07-12T12:00:00Z", "en"));
  });

  it("uses locale plural rules including Ukrainian few and many forms", () => {
    const forms = { one: "{count} задача", few: "{count} задачі", many: "{count} задач", other: "{count} задачі" };
    expect(plural(1, forms, "uk")).toBe("1 задача");
    expect(plural(2, forms, "uk")).toBe("2 задачі");
    expect(plural(5, forms, "uk")).toBe("5 задач");
  });
});
