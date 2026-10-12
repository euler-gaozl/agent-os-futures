import assert from 'node:assert/strict';
import React from 'react';
import test from 'node:test';
import {renderToStaticMarkup} from 'react-dom/server';
import {Answer} from '../src/markdown';

test('research tables and source links render as readable markup',()=>{
 const html=renderToStaticMarkup(<Answer>{'## 库存\n\n| 日期 | 数量 |\n|---|---|\n| 10月8日 | 105 |\n\n[原文](https://www.stats.gov.cn/)'}</Answer>);
 assert.match(html,/<table>/);assert.match(html,/<h2>库存<\/h2>/);assert.match(html,/rel="noopener noreferrer"/);assert.match(html,/href="https:\/\/www.stats.gov.cn\/"/);
});
test('untrusted model output cannot execute HTML or load tracking images',()=>{
 const html=renderToStaticMarkup(<Answer>{'<script>alert(1)</script>\n\n<img src="https://tracking.invalid/pixel" onerror="bad()">\n\n![external image](https://tracking.invalid/image)\n\n[unsafe](javascript:alert%281%29)\n\n[file](file:///etc/passwd)'}</Answer>);
 assert.doesNotMatch(html,/<script|<img|javascript:|file:\/\/|tracking.invalid/);assert.match(html,/external image/);assert.match(html,/unsafe/);
});
