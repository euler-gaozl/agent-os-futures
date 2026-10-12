// Deliberately prints paths only, never matching values or source lines.
import {execFileSync} from 'node:child_process';
import fs from 'node:fs';
const files=execFileSync('git',['ls-files','-z'],{encoding:'utf8'}).split('\0').filter(Boolean);
const forbidden=/^(?:\.state|credentials|secrets|data|artifacts|logs)\/|(?:^|\/)\.env(?!\..*example$|\.example$)(?:\.|$)|\.(?:pem|key|dump|db|sqlite3?)$/i;
const keyPattern=/\bsk-[A-Za-z0-9_-]{24,}\b/;
const failed=[];
for(const file of files){if(forbidden.test(file)){failed.push(file);continue}if(!fs.existsSync(file))continue;const bytes=fs.readFileSync(file);if(keyPattern.test(bytes.toString('utf8')))failed.push(file)}
if(failed.length){console.error('Potential secret/private files detected:\n'+failed.join('\n'));process.exitCode=1}else console.log(`Secret guard passed for ${files.length} tracked files.`);
