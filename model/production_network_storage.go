package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MySQL DDL commits implicitly, so schema installation is deliberately outside
// the data transaction. Runtime accounting and admission always run in triggers.
func initNetworkProductionStorage(db *gorm.DB) error {
	migration := db
	if db.Dialector.Name() == "mysql" {
		migration = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin")
	}
	if err := migration.AutoMigrate(&ProductionTelemetryMinute{}, &ProductionAttemptRecord{}, &ProductionStorageState{}, &ProductionRetentionGap{}); err != nil {
		return err
	}
	for table, columns := range map[string][]string{
		"production_attempt_records":   {"id", "batch_id"},
		"production_telemetry_minutes": {"batch_id"},
		"production_retention_gaps":    {"batch_id"},
	} {
		for _, column := range columns {
			var sql string
			if db.Dialector.Name() == "mysql" {
				sql = "ALTER TABLE " + table + " MODIFY " + column + " varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL"
				if table == "production_attempt_records" && column == "batch_id" {
					sql += " DEFAULT ''"
				}
			} else {
				sql = "ALTER TABLE " + table + " ALTER COLUMN " + column + " TYPE varchar(64) COLLATE \"C\""
			}
			if err := db.Exec(sql).Error; err != nil {
				return err
			}
		}
	}
	var totals struct{ Rows, Bytes int64 }
	if err := db.Raw("SELECT COUNT(*) AS " + productionQuote(db, "rows") + ", COALESCE(SUM(octet_length(payload)),0) AS bytes FROM production_attempt_records").Scan(&totals).Error; err != nil {
		return err
	}
	state := ProductionStorageState{ID: 1, Rows: totals.Rows, Bytes: totals.Bytes, MaxRows: 100000, MaxBytes: 128 << 20}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return err
	}
	if err := installProductionTriggers(db); err != nil {
		return err
	}
	for _, index := range []struct{ name, columns, predicate string }{
		{"idx_production_pending", "next_attempt_at,minute,id", "delivered_at = 0"},
		{"idx_production_minute_cursor", "minute,id", ""},
		{"idx_production_publication", "batch_id,minute", ""},
	} {
		if db.Migrator().HasIndex(&ProductionAttemptRecord{}, index.name) {
			continue
		}
		columns, predicate := index.columns, index.predicate
		if db.Dialector.Name() == "mysql" && predicate != "" {
			columns, predicate = "delivered_at,"+columns, ""
		}
		sql := "CREATE INDEX " + index.name + " ON production_attempt_records(" + columns + ")"
		if predicate != "" {
			sql += " WHERE " + predicate
		}
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	return nil
}

func installProductionTriggers(db *gorm.DB) error {
	if db.Dialector.Name() == "postgres" {
		if err := db.Exec(`CREATE OR REPLACE FUNCTION production_storage_account() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE s production_storage_states%ROWTYPE;
BEGIN
 IF TG_WHEN = 'BEFORE' THEN
  SELECT * INTO STRICT s FROM production_storage_states WHERE id=1 FOR UPDATE;
  IF TG_OP = 'INSERT' THEN
   IF NOT EXISTS (SELECT 1 FROM production_attempt_records WHERE id=NEW.id) AND
    (s.rows >= s.max_rows OR s.bytes + octet_length(NEW.payload) > s.max_bytes) THEN
    RAISE EXCEPTION 'production storage capacity exceeded' USING ERRCODE='23514';
   END IF;
  ELSIF TG_OP = 'UPDATE' AND s.bytes + octet_length(NEW.payload) - octet_length(OLD.payload) > s.max_bytes THEN
   RAISE EXCEPTION 'production storage capacity exceeded' USING ERRCODE='23514';
  END IF;
 ELSE
  IF TG_OP = 'INSERT' THEN
   UPDATE production_storage_states SET rows=rows+1, bytes=bytes+octet_length(NEW.payload) WHERE id=1;
  ELSIF TG_OP = 'DELETE' THEN
   UPDATE production_storage_states SET rows=rows-1, bytes=bytes-octet_length(OLD.payload) WHERE id=1;
  ELSE
   UPDATE production_storage_states SET bytes=bytes+octet_length(NEW.payload)-octet_length(OLD.payload) WHERE id=1;
  END IF;
 END IF;
 IF TG_OP = 'DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END $$`).Error; err != nil {
			return err
		}
		for _, timing := range []string{"BEFORE", "AFTER"} {
			name := "production_storage_" + strings.ToLower(timing)
			// PostgreSQL 15 supports atomic trigger replacement on repeated startup.
			if err := db.Exec("CREATE OR REPLACE TRIGGER " + name + " " + timing + " INSERT OR UPDATE OR DELETE ON production_attempt_records FOR EACH ROW EXECUTE FUNCTION production_storage_account()").Error; err != nil {
				return err
			}
		}
		return nil
	}
	triggers := []struct{ name, event, body string }{
		{"production_storage_admission", "BEFORE INSERT", "DECLARE n,b,mn,mb BIGINT; SELECT `rows`,bytes,max_rows,max_bytes INTO n,b,mn,mb FROM production_storage_states WHERE id=1 FOR UPDATE; IF NOT EXISTS (SELECT 1 FROM production_attempt_records WHERE id=NEW.id) AND (n>=mn OR b+octet_length(NEW.payload)>mb) THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='production storage capacity exceeded'; END IF;"},
		{"production_storage_insert", "AFTER INSERT", "UPDATE production_storage_states SET `rows`=`rows`+1,bytes=bytes+octet_length(NEW.payload) WHERE id=1;"},
		{"production_storage_update_admission", "BEFORE UPDATE", "DECLARE b,mb BIGINT; SELECT bytes,max_bytes INTO b,mb FROM production_storage_states WHERE id=1 FOR UPDATE; IF b+octet_length(NEW.payload)-octet_length(OLD.payload)>mb THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='production storage capacity exceeded'; END IF;"},
		{"production_storage_update", "AFTER UPDATE", "UPDATE production_storage_states SET bytes=bytes+octet_length(NEW.payload)-octet_length(OLD.payload) WHERE id=1;"},
		{"production_storage_delete", "AFTER DELETE", "UPDATE production_storage_states SET `rows`=`rows`-1,bytes=bytes-octet_length(OLD.payload) WHERE id=1;"},
	}
	for _, trigger := range triggers {
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema=DATABASE() AND trigger_name=?", trigger.name).Scan(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if err := db.Exec(fmt.Sprintf("CREATE TRIGGER %s %s ON production_attempt_records FOR EACH ROW BEGIN %s END", trigger.name, trigger.event, trigger.body)).Error; err != nil {
			return err
		}
	}
	return nil
}
