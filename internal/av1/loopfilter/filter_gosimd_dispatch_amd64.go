// SPDX-License-Identifier: BSD-2-Clause
//
// See LICENSE for the BSD-2-Clause grant.

//go:build goexperiment.simd && amd64 && !purego

package loopfilter

import "simd/archsimd"

// init binds Go-native SIMD deblocking on amd64 using go-vpx-style runtime
// feature checks: AVX2 kernels when archsimd.X86.AVX2() (AVX-512-specific
// variants are not wired here because several narrows are AVX2-only by design).
func init() {
	if archsimd.X86.AVX2() {
		filter4EdgeImpl = filter4EdgeSIMD
		filter4Edge16Impl = filter4Edge16SIMD
		filter6EdgeImpl = filter6EdgeSIMD
		filter8EdgeImpl = filter8EdgeSIMD
		filter14EdgeImpl = filter14EdgeSIMD
	}
}
