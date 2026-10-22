import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, translationKeys } from './lib.mjs';
const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'signature'); fs.mkdirSync(out, { recursive: true });
const report = new Report('detached-signature');
const fixture = path.resolve('../testdata/signature');
const labels = {
  en: { title: 'Verify Signature', file: 'Exported file', cms: 'Detached signature (.p7s)', verify: 'Verify files', valid: 'Signature valid — trust unknown', modified: 'File differs from the signed bytes', close: 'Close' },
  'pt-BR': { title: 'Verificar assinatura', file: 'Arquivo exportado', cms: 'Assinatura separada (.p7s)', verify: 'Verificar arquivos', valid: 'Assinatura válida — confiança desconhecida', modified: 'O arquivo difere dos bytes assinados', close: 'Fechar' },
  es: { title: 'Verificar firma', file: 'Archivo exportado', cms: 'Firma separada (.p7s)', verify: 'Verificar archivos', valid: 'Firma válida — confianza desconocida', modified: 'El archivo difiere de los bytes firmados', close: 'Cerrar' },
};
const browser = await launchBrowser();
try {
  for (const [lang, l] of Object.entries(labels)) {
    for (const [theme, width, height] of [['light',1366,768],['dark',1024,600]]) {
      const page = await browser.newPage({ viewport: { width, height } });
      await page.goto(await launchURL(dir)); await page.waitForLoadState('networkidle');
      const state = await api(page,'GET','/api/state');
      await api(page,'PUT','/api/profile/settings',{language:lang,theme,onboarding:{tutorial:true,ls:true,turbo:true},whatsNewSeen:state.version});
      await page.reload(); await page.waitForLoadState('networkidle');
      const before = await api(page,'GET','/api/files');
      await page.locator('.avatar-btn').click();
      await page.getByRole('menuitem',{name:l.title,exact:true}).click();
      report.check(`${lang}/${theme}: verification starts disabled`,await page.getByRole('button',{name:l.verify,exact:true}).isDisabled());
      await page.getByLabel(l.file,{exact:true}).setInputFiles(path.join(fixture,'invented.csv'));
      await page.getByLabel(l.cms,{exact:true}).setInputFiles(path.join(fixture,'invented.p7s'));
      await page.getByRole('button',{name:l.verify,exact:true}).click();
      await page.getByText(l.valid,{exact:true}).waitFor();
      report.check(`${lang}/${theme}: shows trust-unknown independently generated CMS`,await page.getByText(l.valid,{exact:true}).isVisible());
      const problems = await layoutProblems(page,translationKeys());
      report.check(`${lang}/${theme}: readable verification result`,problems.length===0,problems.join('; '));
      await page.screenshot({path:path.join(out,`verify-${lang}-${theme}.png`)});
      await page.getByLabel(l.file,{exact:true}).setInputFiles({name:'changed.csv',mimeType:'text/csv',buffer:Buffer.from('invented but changed')});
      report.check(`${lang}/${theme}: changing a file clears previous result`,await page.getByText(l.valid,{exact:true}).count()===0);
      await page.getByRole('button',{name:l.verify,exact:true}).click();
      await page.getByText(l.modified,{exact:true}).waitFor();
      report.check(`${lang}/${theme}: changed bytes reported separately`,await page.getByText(l.modified,{exact:true}).isVisible());
      report.check(`${lang}/${theme}: no library import`,JSON.stringify(before)===JSON.stringify(await api(page,'GET','/api/files')));
      await page.getByRole('button',{name:l.close,exact:true}).last().click();
      await page.locator('.avatar-btn').click(); await page.getByRole('menuitem',{name:l.title,exact:true}).click();
      report.check(`${lang}/${theme}: closing drops file handles and results`,await page.getByRole('button',{name:l.verify,exact:true}).isDisabled()&&await page.getByLabel(l.file,{exact:true}).inputValue()==='');
      await page.close();
    }
  }
} catch(e) { report.check('signature workflow completes',false,e.message.split('\n')[0]);fs.writeFileSync(path.join(out,'failure.txt'),e.stack||String(e)); }
finally { await browser.close(); }
process.exit(report.finish(out));
