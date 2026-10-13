import assert from 'node:assert/strict';
import React from 'react';
import test from 'node:test';
import {renderToStaticMarkup} from 'react-dom/server';
import {Answer,sourceDocument} from '../src/markdown';
import {sourceDetails,uniqueSources,type Evidence} from '../src/evidence';

const record:Evidence={operation_id:'run/read',run_id:'run',document_id:'document-1',title:'库存资料',source:'upload:document.md',kind:'evidence',preview:'来源：https://source.invalid/archive\n发布日期：2026-09-29\n统计范围：9月25日，主要油厂',preview_truncated:false,source_truncated:false,read_offset:0,read_end:400,has_more:false,recorded_at:'2026-10-13T00:00:00Z'};

test('archived document references become controlled buttons, not external navigation',()=>{
 const html=renderToStaticMarkup(<Answer onSource={()=>{}}>{'[库存来源](source://document/document-1) [外部来源](https://source.invalid/) [伪造](source://document/../../other)'}</Answer>);
 assert.match(html,/<button[^>]*class="source-citation"/);assert.match(html,/库存来源/);
 assert.doesNotMatch(html,/href="source:|\.\.\/|onclick=/);assert.match(html,/href="https:\/\/source.invalid\/"/);
 for(const url of ['source://document/../x','source://document/x?token=y','source://document/x#y','source://other/x','SOURCE://document/x','source://document/'+ 'x'.repeat(121)]) assert.equal(sourceDocument(url),null);
 assert.equal(sourceDocument('source://document/document-1'),'document-1');
});
test('source labels come from retained first-page material; executable and credential URLs are rejected',()=>{
 assert.deepEqual(sourceDetails(record),{url:'https://source.invalid/archive',published:'2026-09-29',period:'9月25日，主要油厂'});
 assert.deepEqual(sourceDetails({...record,read_offset:400}),{url:'',published:'',period:''});
 for(const source of ['javascript:alert(1)','file:///private','https://user:secret@source.invalid/','data:text/html,test']) assert.equal(sourceDetails({...record,preview:'',source}).url,'');
 assert.equal(sourceDetails({...record,preview:'',source:'https://source.invalid/'}).url,'https://source.invalid/');
});
test('listing deduplicates documents without losing the first-page provenance when it arrives later',()=>{
 const later={...record,operation_id:'run/read2',read_offset:400,preview:'后半段'};
 const second={...record,document_id:'document-2',operation_id:'run/read3'};
 assert.deepEqual(uniqueSources([later,second,record]),[record,second]);
 assert.deepEqual(uniqueSources([record,later]),[record]);
});
test('truncated metadata is never promoted to a complete source URL or date',()=>{
 const preview=('来源：https://source.invalid/'+ 'a'.repeat(600)).slice(0,512);
 assert.equal(sourceDetails({...record,preview,preview_truncated:true}).url,'');
 assert.equal(sourceDetails({...record,preview,preview_truncated:true,source:'https://source.invalid/complete'}).url,'https://source.invalid/complete');
 assert.equal(sourceDetails({...record,preview:'',source:'https://source.invalid/'+ 'a'.repeat(2025),source_truncated:true}).url,'');
 const boundary={...record,preview:'来源：https://source.invalid/complete\n发布日期：2026-09',preview_truncated:true};
 assert.equal(sourceDetails(boundary).url,'https://source.invalid/complete');assert.equal(sourceDetails(boundary).published,'');
 assert.equal(sourceDetails({...boundary,preview:boundary.preview+'\n'}).published,'2026-09');
 // The original document_read may itself have stopped mid-line before the
 // API preview limit was reached. Its partial final line is not provenance.
 const short={...record,preview:preview.slice(0,300),has_more:true,preview_truncated:false};
 assert.equal(sourceDetails(short).url,'');
 assert.equal(sourceDetails({...short,source:'https://source.invalid/complete'}).url,'https://source.invalid/complete');
 assert.equal(sourceDetails({...boundary,has_more:true,preview_truncated:false}).published,'');
});
