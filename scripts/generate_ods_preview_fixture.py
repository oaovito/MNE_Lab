#!/usr/bin/env python3
"""Generate invented ODF 1.3 cells, never experimental/instrument data.

Standard-library ZIP writer is independent of the product's Go XML reader.
Fixed timestamps, stored first MIME part and explicit source values make this
fixture reproducible. The OASIS schema was checked separately during development.
"""
from pathlib import Path
from zipfile import ZipFile, ZipInfo, ZIP_STORED, ZIP_DEFLATED
from io import BytesIO

OFFICE = 'urn:oasis:names:tc:opendocument:xmlns:office:1.0'
TABLE = 'urn:oasis:names:tc:opendocument:xmlns:table:1.0'
TEXT = 'urn:oasis:names:tc:opendocument:xmlns:text:1.0'
MANIFEST = 'urn:oasis:names:tc:opendocument:xmlns:manifest:1.0'
MIME = 'application/vnd.oasis.opendocument.spreadsheet'
content = f'''<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="{OFFICE}" xmlns:table="{TABLE}" xmlns:text="{TEXT}" office:version="1.3">
<office:body><office:spreadsheet>
<table:table table:name="Invented literal cells"><table:table-column table:number-columns-repeated="6"/>
<table:table-row><table:table-cell office:value-type="string"><text:p>Effective Diameter (nm)</text:p></table:table-cell><table:table-cell office:value-type="string"><text:p>Original display</text:p></table:table-cell></table:table-row>
<table:table-row table:number-rows-repeated="2">
<table:table-cell office:value-type="float" office:value="1.2300"><text:p>1,2300</text:p></table:table-cell>
<table:table-cell table:number-columns-repeated="2" office:value-type="percentage" office:value="0.50"><text:p>50%</text:p></table:table-cell>
<table:table-cell office:value-type="date" office:date-value="2026-01-02T03:04:05"><text:p>02/01/2026</text:p></table:table-cell>
<table:table-cell office:value-type="boolean" office:boolean-value="false"><text:p>FALSE</text:p></table:table-cell>
<table:table-cell office:value-type="time" office:time-value="PT1H2M3S"><text:p>01:02:03</text:p></table:table-cell>
</table:table-row>
<table:table-row><table:table-cell office:value-type="string"><text:p> alpha  <text:span> beta</text:span><text:s text:c="2"/>gamma<text:tab/>delta<text:line-break/>end </text:p><text:p>second</text:p></table:table-cell></table:table-row>
<table:table-row><table:table-cell/><table:table-cell office:value-type="string" office:string-value=""/><table:table-cell office:value-type="float" office:value="0.0000"/></table:table-row>
</table:table>
<table:table table:name="Invented bounded cells"><table:table-column table:number-columns-repeated="22"/><table:table-row table:number-rows-repeated="21"><table:table-cell office:value-type="string"><text:p>{'é'*200}</text:p></table:table-cell><table:table-cell table:number-columns-repeated="21"/></table:table-row></table:table>
</office:spreadsheet></office:body></office:document-content>
'''
manifest = f'''<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="{MANIFEST}" manifest:version="1.3"><manifest:file-entry manifest:full-path="/" manifest:media-type="{MIME}" manifest:version="1.3"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>'''
out = BytesIO()
with ZipFile(out,'w') as z:
 for name, value in [('mimetype',MIME),('META-INF/manifest.xml',manifest),('content.xml',content)]:
  info = ZipInfo(name,(2026,1,1,0,0,0));info.compress_type=ZIP_STORED if name=='mimetype' else ZIP_DEFLATED
  info.external_attr=0o600 << 16
  z.writestr(info,value.encode('utf-8'))
root=Path(__file__).resolve().parents[1]
(root/'testdata/ods-preview/invented.ods').write_bytes(out.getvalue())
