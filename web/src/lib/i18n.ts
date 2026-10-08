// Scientific labels shared with the core (axes, parameters, columns…): one
// source of truth for the interface, the figures and the exports.
import goEn from '../../../internal/i18n/locales/en.json';
import goPt from '../../../internal/i18n/locales/pt-BR.json';
import goEs from '../../../internal/i18n/locales/es.json';

export type Lang = 'en' | 'pt-BR' | 'es';
export const LANGS: Lang[] = ['en', 'pt-BR', 'es'];
export const LANG_NAMES: Record<Lang, string> = { en: 'English', 'pt-BR': 'Português (Brasil)', es: 'Español' };

export type Table = Record<string, readonly [string, string, string]>;
const shared: Table = {};
for (const k of Object.keys(goEn)) {
  const g = (m: Record<string, string>) => m[k] || (goEn as Record<string, string>)[k];
  shared[k] = [g(goEn), g(goPt), g(goEs)];
}
const catalog: Table = { ...shared };

/** addCatalogs registers interface texts; each entry point loads only what it shows. */
export function addCatalogs(...tables: Table[]) {
  for (const tb of tables) Object.assign(catalog, tb);
}

let current: Lang = 'en';
let idx = 0;

export function setLang(l: string) {
  current = (LANGS.includes(l as Lang) ? l : 'en') as Lang;
  idx = LANGS.indexOf(current);
  document.documentElement.lang = current;
}

export const lang = () => current;

/**
 * t translates a neutral key; {name} placeholders are replaced from vars and
 * {n:one/other} picks the singular or plural word from the number n.
 */
export function t(key: string, vars?: Record<string, string | number>): string {
  const row = catalog[key];
  let s = row ? row[idx] || row[0] : key;
  if (vars) {
    s = s.replace(/\{(\w+):([^{}/]*)\/([^{}]*)\}/g, (_, v: string, one: string, other: string) => (Number(vars[v]) === 1 ? one : other));
    for (const k in vars) s = s.split('{' + k + '}').join(String(vars[k]));
  }
  return s;
}

/** has reports whether a key exists (for optional keys such as error codes). */
export const has = (key: string) => key in catalog;

/** errText turns a stable error identifier from the core into a message. */
export function errText(code: string): string {
  if (has('err.' + code)) return t('err.' + code);
  const area = code.split('.')[0];
  if (has('err.' + area)) return t('err.' + area);
  return t('err.generic');
}

export const keys = () => Object.keys(catalog);
