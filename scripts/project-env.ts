// Resolve once through the vendored shell helper before spawning Bun/Go children.
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
const helper = fileURLToPath(new URL('./project-env.sh', import.meta.url));
const names = ['PROJECT_TMP_ROOT','TMPDIR','TMP','TEMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOPATH','GOBIN','BUN_INSTALL_CACHE_DIR','npm_config_cache','PLAYWRIGHT_BROWSERS_PATH','XDG_CACHE_HOME'];
const command = 'source "$1" || exit 1; ' + names.map(name=>`printf '%s\\0' '${name}='"$${name}"`).join('; ');
const output=execFileSync('bash',['-c',command,'project-env',helper],{encoding:'utf8'});
for(const row of output.split('\0').filter(Boolean)){const at=row.indexOf('=');process.env[row.slice(0,at)]=row.slice(at+1);}
export const projectRoot=process.env.PROJECT_TMP_ROOT!;
export const projectBuild=join(projectRoot,'build');
