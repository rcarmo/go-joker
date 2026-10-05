"""Environment setup only (no Python profiling), shared with Make/Bun/shell."""
import os
import subprocess
from pathlib import Path

_project_env_helper = Path(__file__).resolve().parents[1] / "scripts/project-env.sh"
# exec-loading this module from benchmarks preserves the benchmark's __file__.
if not _project_env_helper.exists():
    _project_env_helper = Path(__file__).resolve().with_name("project-env.sh")

def configure():
    names = ["PROJECT_TMP_ROOT", "TMPDIR", "TMP", "TEMP", "GOTMPDIR", "GOCACHE", "GOMODCACHE", "GOPATH", "GOBIN", "BUN_INSTALL_CACHE_DIR", "npm_config_cache", "PYTHONPYCACHEPREFIX", "PLAYWRIGHT_BROWSERS_PATH", "XDG_CACHE_HOME"]
    command = 'source "$1" || exit 1; ' + '; '.join("printf '%s\\0' '"+name+"='\"$"+name+"\"" for name in names)
    output = subprocess.check_output(["bash", "-c", command, "project-env", str(_project_env_helper)], text=True)
    for row in output.split("\0"):
        if row:
            name, value = row.split("=", 1)
            os.environ[name] = value
