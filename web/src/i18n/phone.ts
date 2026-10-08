// The texts the phone interface shows (help and export stay on the computer,
// which keeps the phone download small).
import { addCatalogs } from '../lib/i18n';
import { core } from './core';
import { errors } from './errors';
import { ls } from './ls';

addCatalogs(core, ls, errors);
