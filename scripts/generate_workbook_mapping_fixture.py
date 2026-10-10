#!/usr/bin/env python3
"""Generate invented OOXML bytes independently of Excel/Excelize."""
from pathlib import Path
from zipfile import ZipFile, ZipInfo, ZIP_STORED
from xml.sax.saxutils import escape
import hashlib
root=Path(__file__).resolve().parents[1]
ns='http://schemas.openxmlformats.org/spreadsheetml/2006/main'
def text(cell,value): return f'<c r="{cell}" t="inlineStr"><is><t>{escape(value)}</t></is></c>'
def row(n,values): return f'<row r="{n}">{values}</row>'
rows=[row(1,text('A1','INVENTED DATA — no instrument reference')),row(2,text('A2','opaque sample')+text('B2','unknown size (um)')+text('C2','unknown count')+text('D2','unknown text')),
 row(3,'<c r="A3" t="s"><v>0</v></c><c r="B3" s="1"><v>1.2300</v></c><c r="C3"><v>0.0000</v></c>'+text('D3','1,2500')),
 row(4,text('A4','Missing')),row(5,text('A5','Zero')+'<c r="B5"><v>0.0000</v></c>')]
# Physical row 6 is omitted; it remains an absent observation when selected.
for n in range(7,29): rows.append(row(n,text(f'A{n}','Invented')+f'<c r="B{n}"><v>{n-5}.0000</v></c>'))
parts={
'[Content_Types].xml':'<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/><Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/></Types>',
'_rels/.rels':'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>',
'xl/workbook.xml':f'<workbook xmlns="{ns}" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Raw Data" sheetId="1" r:id="rId1"/><sheet name="Other" sheetId="2" r:id="rId2"/></sheets></workbook>',
'xl/_rels/workbook.xml.rels':'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'+''.join(f'<Relationship Id="rId{n}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet{n}.xml"/>' for n in (1,2))+'<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/></Relationships>',
'xl/worksheets/sheet1.xml':f'<worksheet xmlns="{ns}"><sheetData>'+''.join(rows)+'</sheetData></worksheet>',
'xl/worksheets/sheet2.xml':f'<worksheet xmlns="{ns}"><sheetData>'+row(1,text('A1','Excluded opaque sheet'))+'</sheetData></worksheet>',
'xl/styles.xml':f'<styleSheet xmlns="{ns}"><fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills><borders count="1"><border/></borders><cellStyleXfs count="1"><xf/></cellStyleXfs><cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/><xf numFmtId="2" fontId="0" fillId="0" borderId="0"/></cellXfs></styleSheet>',
'xl/sharedStrings.xml':f'<sst xmlns="{ns}" count="1" uniqueCount="1"><si><t xml:space="preserve">Synthetic\nsample</t></si></sst>'}
file=root/'testdata/workbook-mapping/invented.xlsx';file.parent.mkdir(parents=True,exist_ok=True)
with ZipFile(file,'w',compression=ZIP_STORED) as z:
 for name,body in parts.items():
  info=ZipInfo(name,date_time=(2000,1,1,0,0,0)); info.external_attr=0o600<<16; z.writestr(info,body.encode())
print(hashlib.sha256(file.read_bytes()).hexdigest())
