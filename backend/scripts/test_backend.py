#!/usr/bin/env python3
"""Run Go integration tests against a dedicated DB on the project's MySQL container."""
import os
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[1]
values = {}
for line in (root / '.env').read_text().splitlines():
    if line and not line.startswith('#') and '=' in line:
        key, value = line.split('=', 1)
        values[key] = value.strip().strip('"').strip("'")
# The test schema is isolated from drift_bottle and uses the existing local app user.
bootstrap = "CREATE DATABASE IF NOT EXISTS drift_backend_test CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci; GRANT ALL ON drift_backend_test.* TO 'drift_app'@'%';"
command = ['docker', 'compose', 'exec', '-T', 'mysql', 'sh', '-c',
           'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --default-character-set=utf8mb4']
subprocess.run(command, cwd=root, input=bootstrap, text=True, check=True)
existing = subprocess.run(command, cwd=root, input="SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='drift_backend_test' AND table_name='users';", text=True, capture_output=True, check=True)
if existing.stdout.strip().splitlines()[-1] == '0':
    schema = 'USE drift_backend_test;\n' + (root / 'migrations/000001_initial.up.sql').read_text()
    subprocess.run(command, cwd=root, input=schema, text=True, check=True)
slot_columns = subprocess.run(
    command, cwd=root,
    input="SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema='drift_backend_test' AND table_name='active_search_slots' AND index_name='PRIMARY';",
    text=True, capture_output=True, check=True)
if slot_columns.stdout.strip().splitlines()[-1] == '1':
    migration = 'USE drift_backend_test;\n' + (root / 'migrations/000002_multiple_active_bottles.up.sql').read_text()
    subprocess.run(command, cwd=root, input=migration, text=True, check=True)
profile_columns = subprocess.run(
    command, cwd=root,
    input="SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='drift_backend_test' AND table_name='users' AND column_name='display_name';",
    text=True, capture_output=True, check=True)
if profile_columns.stdout.strip().splitlines()[-1] == '0':
    migration = 'USE drift_backend_test;\n' + (root / 'migrations/000003_zhihu_login.up.sql').read_text()
    subprocess.run(command, cwd=root, input=migration, text=True, check=True)
environment = dict(os.environ)
environment['TEST_MYSQL_DSN'] = f"drift_app:{values['MYSQL_PASSWORD']}@tcp(127.0.0.1:{values.get('MYSQL_PORT', '3307')})/drift_backend_test?parseTime=true&loc=UTC"
environment['GOCACHE'] = '/tmp/drift-go-cache'
subprocess.run(['go', 'test', '-race', '-count=1', './...'], cwd=root, env=environment, check=True)

environment['TEST_DATABASE'] = 'drift_backend_test'
subprocess.run(['python3', '-m', 'unittest', 'discover', '-s', 'tests', '-p', 'test_database.py', '-v'], cwd=root, env=environment, check=True)
