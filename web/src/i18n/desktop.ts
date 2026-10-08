// Every interface text of the desktop window.
import { addCatalogs } from '../lib/i18n';
import { core } from './core';
import { errors } from './errors';
import { exp } from './export';
import { help } from './help';
import { ls } from './ls';

addCatalogs(core, ls, exp, help, errors);
