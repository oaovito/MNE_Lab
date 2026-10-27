// Every interface text of the desktop window.
import { addCatalogs } from '../lib/i18n';
import { core } from './core';
import { design } from './design';
import { errors } from './errors';
import { exp } from './export';
import { help } from './help';
import { ls } from './ls';
import { statistics } from './statistics';
import { tutorial } from './tutorial';

addCatalogs(core, ls, exp, help, errors, tutorial, design, statistics);
