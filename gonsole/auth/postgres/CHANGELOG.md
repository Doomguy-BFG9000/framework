# Changelog

All notable changes to the `gonsole/auth/postgres` module are documented
in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
module follows [Semantic Versioning](https://semver.org/). While at
v0.x, minor releases may contain breaking changes.

Releases of this module are tagged `gonsole/auth/postgres/vX.Y.Z`.

## [Unreleased]

### Added

- `Stores`, the account and record stores of a call on a pool of the database setting, closed after the call, for `auth.Config.Stores`.
- `Migration` and `RecordMigration`, the two schema steps, each under goose's migration lock.
- The records keep the version table of `gonsole/auth`, `gonsole.goose_db_version`.
- `NewRecordStore`, the record store of the `gonsole.records` table, which lists no record for a limit below zero.
- A constraint, lock or serialization failure of the record store or the records step carries dbkit's error class, such as `dbkit.ErrUnique`.
- An address pgx cannot parse fails `Stores` and the records step with one message that holds no part of the address.
