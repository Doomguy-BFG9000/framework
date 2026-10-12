# Changelog

All notable changes to the `gonsole/auth/sqlite` module are documented
in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
module follows [Semantic Versioning](https://semver.org/). While at
v0.x, minor releases may contain breaking changes.

Releases of this module are tagged `gonsole/auth/sqlite/vX.Y.Z`.

## [Unreleased]

### Added

- `Stores`, the account and record stores of a call on the run's one SQLite handle, for `auth.Config.Stores`.
- `Migration` and `RecordMigration` with `MigrateOptions`, the two schema steps, each under the file's migration lock.
- The records keep their own version table, `gonsole_goose_db_version`.
- `NewRecordStore`, the record store of the `gonsole_records` table, whose arguments and flags must be valid JSON.
- A busy, locked or constraint failure of the record store or the records step carries dbkit's error class.
