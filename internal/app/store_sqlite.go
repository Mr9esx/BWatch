package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

const stateDBVersion = 1

func defaultState() State {
	return State{
		SchemaVersion: 3,
		Creators:      []Creator{},
		Tasks:         []Task{},
		Events:        []Event{},
		Settings: Settings{
			Concurrency: 1, Interval: 10, Quality: "best", SaveThumbnail: true,
			DownloadMinInterval: 1, DownloadMaxInterval: 3,
			BilibiliMinInterval: 1, BilibiliMaxInterval: 3,
		},
	}
}

func openStateDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.Exec(`PRAGMA busy_timeout = 5000`); err == nil {
		_, err = db.Exec(`PRAGMA journal_mode = WAL`)
	}
	if err == nil {
		_, err = db.Exec(`PRAGMA synchronous = FULL`)
	}
	if err == nil {
		err = migrateStateDB(db)
	}
	if err == nil {
		err = os.Chmod(path, 0600)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migrateStateDB(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > stateDBVersion {
		return fmt.Errorf("数据库版本 %d 高于当前程序支持版本 %d，请使用较新版本程序", version, stateDBVersion)
	}
	for version < stateDBVersion {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		next := version + 1
		switch next {
		case 1:
			_, err = tx.Exec(`CREATE TABLE app_state (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				payload TEXT NOT NULL,
				updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`)
		default:
			err = fmt.Errorf("缺少数据库迁移版本 %d", next)
		}
		if err == nil {
			_, err = tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, next))
		}
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		version = next
	}
	return nil
}

func (a *App) loadState() error {
	var payload string
	err := a.stateDB.QueryRow(`SELECT payload FROM app_state WHERE id = 1`).Scan(&payload)
	if err == nil {
		if err = json.Unmarshal([]byte(payload), &a.state); err != nil {
			return fmt.Errorf("读取 SQLite 状态失败：%w", err)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	legacy, err := os.ReadFile(a.statePath)
	if err == nil {
		if err = json.Unmarshal(legacy, &a.state); err != nil {
			return fmt.Errorf("读取旧版 state.json 失败：%w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
