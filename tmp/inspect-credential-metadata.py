import json
import sqlite3
import sys
from datetime import datetime
from pathlib import Path

database = Path(__file__).resolve().parents[1] / "starhub.db"
connection = sqlite3.connect(database.as_uri() + "?mode=ro", uri=True)
connection.execute("PRAGMA query_only=ON")
report = {"database": str(database), "tables": {}}
if "--backup" in sys.argv:
    backup = database.parent / ".p6-work" / "credential-recovery" / datetime.now().strftime("%Y%m%d-%H%M%S-%f") / "starhub.db"
    backup.parent.mkdir(parents=True)
    destination = sqlite3.connect(backup)
    connection.backup(destination)
    destination.close()
    report["backup"] = str(backup)
for table in ("channels", "tokens"):
    records = []
    for identifier, value in connection.execute(f'SELECT id, "key" FROM "{table}" ORDER BY id'):
        value = value or ""
        if value.startswith("enc:v2:"):
            parts = value.split(":", 3)
            kind = "envelope-v2"
            key_id = parts[2]
        elif value.startswith("enc:"):
            kind, key_id = "legacy-encrypted", "legacy"
        elif value:
            kind, key_id = "plaintext", None
        else:
            kind, key_id = "empty", None
        records.append({"id": identifier, "format": kind, "wrapping_key_id": key_id})
    report["tables"][table] = {"count": len(records), "credentials": records}
connection.close()
print(json.dumps(report, ensure_ascii=True, indent=2))
