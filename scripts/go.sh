#!/usr/bin/env bash
# Match go-vpx: default GOEXPERIMENT=simd for local builds and tests.
export GOEXPERIMENT="${GOEXPERIMENT-simd}"
exec go "$@"
