# libaom AV1 conformance IVF fixtures (minimal set)

These files are excerpted from the [libaom AV1 test vector suite](https://storage.googleapis.com/aom-test-data/) (libaom v3.14.0 `test/test_vectors.cc` / `test/test-data.sha1`).

- **Copyright**: BSD-2-Clause (libaom)
- **Patents**: AV1 patent license (AOMedia)

The full corpus is large; this directory commits only the IVF files required for `go test ./...` in a fresh checkout. Additional vectors are downloaded on demand via `internal/av1/testvector.EnsureRemoteFile` and CI may run `.github/scripts/fetch-libaom-ci-fixtures.sh`.
