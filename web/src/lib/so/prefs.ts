import { fileExists, readJSONFile, writeText } from "./nodeio";
import { soPath } from "./root";

type Prefs = Record<string, string>;

function prefsFile(): string {
  return soPath("ui-prefs.json");
}

function load(): Prefs {
  const p = prefsFile();
  if (!fileExists(p)) return {};
  return readJSONFile<Prefs>(p) ?? {};
}

function save(prefs: Prefs) {
  writeText(prefsFile(), JSON.stringify(prefs, null, 2));
}

export function getPref(key: string): string | null {
  const v = load()[key];
  return v === undefined ? null : v;
}

export function getAllPrefs(): Prefs {
  return load();
}

export function setPref(key: string, value: string) {
  const prefs = load();
  prefs[key] = value;
  save(prefs);
}
