// +build !darwin

package blake3

import "github.com/klauspost/cpuid/v2"

var (
	haveAVX2   = cpuid.CPU.Supports(cpuid.AVX2)
	haveAVX512 = cpuid.CPU.Supports(cpuid.AVX512F)
)
