//go:build arm64 && !purego

#include "textflag.h"

// func maxByteNEON(d []byte) byte
// Extreme Byte Search (maximum): 16-byte VUMAX accumulation, scalar tail, then
// a scalar reduction over the 16-lane accumulator (arm64's Go assembler has no
// horizontal-max instruction, so the accumulator is spilled and reduced the
// same way the amd64 AVX2 path reduces its 32-lane accumulator).
TEXT ·maxByteNEON(SB), NOSPLIT, $16-25
	MOVD	d_base+0(FP), R0
	MOVD	d_len+8(FP), R1
	MOVD	$0, R2
	VEOR	V0.B16, V0.B16, V0.B16
	CMP	$16, R1
	BLT	maxTail

maxBlk:
	VLD1.P	16(R0), [V1.B16]
	VUMAX	V1.B16, V0.B16, V0.B16
	SUB	$16, R1, R1
	CMP	$16, R1
	BGE	maxBlk

maxTail:
	CBZ	R1, maxReduce

maxTailLoop:
	MOVBU.P	1(R0), R3
	CMP	R2, R3
	BLS	maxTailNext
	MOVD	R3, R2

maxTailNext:
	SUB	$1, R1, R1
	CBNZ	R1, maxTailLoop

maxReduce:
	VST1	[V0.B16], (RSP)
	MOVD	RSP, R4
	MOVD	$16, R1

maxRl:
	MOVBU.P	1(R4), R3
	CMP	R2, R3
	BLS	maxRlNext
	MOVD	R3, R2

maxRlNext:
	SUB	$1, R1, R1
	CBNZ	R1, maxRl

	MOVB	R2, ret+24(FP)
	RET

// func minByteNEON(d []byte) byte
// Extreme Byte Search (minimum). Mirror of maxByteNEON with VUMIN and an
// accumulator seeded to 0xFF.
TEXT ·minByteNEON(SB), NOSPLIT, $16-25
	MOVD	d_base+0(FP), R0
	MOVD	d_len+8(FP), R1
	MOVD	$0xff, R2
	CBZ	R1, minEmpty
	VMOVI	$0xff, V0.B16
	CMP	$16, R1
	BLT	minTail

minBlk:
	VLD1.P	16(R0), [V1.B16]
	VUMIN	V1.B16, V0.B16, V0.B16
	SUB	$16, R1, R1
	CMP	$16, R1
	BGE	minBlk

minTail:
	CBZ	R1, minReduce

minTailLoop:
	MOVBU.P	1(R0), R3
	CMP	R2, R3
	BHS	minTailNext
	MOVD	R3, R2

minTailNext:
	SUB	$1, R1, R1
	CBNZ	R1, minTailLoop

minReduce:
	VST1	[V0.B16], (RSP)
	MOVD	RSP, R4
	MOVD	$16, R1

minRl:
	MOVBU.P	1(R4), R3
	CMP	R2, R3
	BHS	minRlNext
	MOVD	R3, R2

minRlNext:
	SUB	$1, R1, R1
	CBNZ	R1, minRl

	MOVB	R2, ret+24(FP)
	RET

minEmpty:
	MOVB	$0, ret+24(FP)
	RET

// func indexCmpNEON(d []byte, target byte, op uint64) int
// Range Scan. op selects the comparator: 0=GT, 1=GE, 2=LT, 3=LE. arm64's Go
// assembler exposes no unsigned greater/less compare, so both directions are
// built from packed min/max + equality exactly like the amd64 path:
//   x <= t  <=>  min(x,t) == x        x >= t  <=>  max(x,t) == x
//   x >  t  <=>  ~(x <= t)            x <  t  <=>  ~(x >= t)
// A 16-byte block only tells us whether it holds a hit (mask AND 1, summed via
// VADDV); pinpointing the index within a hit block falls back to a scalar
// scan, since arm64 has no move-mask instruction either.
TEXT ·indexCmpNEON(SB), NOSPLIT, $0-48
	MOVD	d_base+0(FP), R0
	MOVD	d_len+8(FP), R1
	MOVBU	target+24(FP), R2
	MOVD	op+32(FP), R3
	MOVD	$0, R11

	VMOVI	$1, V6.B16
	VMOVI	$0xff, V7.B16
	VMOV	R2, V2.B16

	CMP	$16, R1
	BLT	icTail

icBlk:
	MOVD	R0, R12
	VLD1.P	16(R0), [V0.B16]
	VUMIN	V0.B16, V2.B16, V3.B16
	VUMAX	V0.B16, V2.B16, V4.B16
	VCMEQ	V0.B16, V3.B16, V3.B16
	VCMEQ	V0.B16, V4.B16, V4.B16

	CMP	$0, R3
	BEQ	icBlkGT
	CMP	$1, R3
	BEQ	icBlkGE
	CMP	$2, R3
	BEQ	icBlkLT
	VMOV	V3.B16, V1.B16
	B	icBlkMask

icBlkGT:
	VEOR	V3.B16, V7.B16, V1.B16
	B	icBlkMask

icBlkGE:
	VMOV	V4.B16, V1.B16
	B	icBlkMask

icBlkLT:
	VEOR	V4.B16, V7.B16, V1.B16

icBlkMask:
	VAND	V6.B16, V1.B16, V5.B16
	VADDV	V5.B16, V8
	VMOV	V8.B[0], R9
	CBNZ	R9, icFoundBlk

	ADD	$16, R11, R11
	SUB	$16, R1, R1
	CMP	$16, R1
	BGE	icBlk

icTail:
	CBZ	R1, icNotFound

icTailLoop:
	MOVBU	(R0), R10
	CMP	$0, R3
	BEQ	icTGT
	CMP	$1, R3
	BEQ	icTGE
	CMP	$2, R3
	BEQ	icTLT
	CMP	R2, R10
	BLS	icFoundTail
	B	icTnext

icTGT:
	CMP	R2, R10
	BHI	icFoundTail
	B	icTnext

icTGE:
	CMP	R2, R10
	BHS	icFoundTail
	B	icTnext

icTLT:
	CMP	R2, R10
	BLO	icFoundTail

icTnext:
	ADD	$1, R0, R0
	ADD	$1, R11, R11
	SUB	$1, R1, R1
	CBNZ	R1, icTailLoop
	B	icNotFound

icFoundBlk:
	MOVD	$0, R13

icFoundBlkLoop:
	MOVBU	(R12)(R13), R10
	CMP	$0, R3
	BEQ	icFBGT
	CMP	$1, R3
	BEQ	icFBGE
	CMP	$2, R3
	BEQ	icFBLT
	CMP	R2, R10
	BLS	icFoundBlkHit
	B	icFoundBlkNext

icFBGT:
	CMP	R2, R10
	BHI	icFoundBlkHit
	B	icFoundBlkNext

icFBGE:
	CMP	R2, R10
	BHS	icFoundBlkHit
	B	icFoundBlkNext

icFBLT:
	CMP	R2, R10
	BLO	icFoundBlkHit

icFoundBlkNext:
	ADD	$1, R13, R13
	B	icFoundBlkLoop

icFoundBlkHit:
	ADD	R13, R11, R11
	B	icRet

icFoundTail:
	B	icRet

icNotFound:
	MOVD	d_len+8(FP), R11

icRet:
	MOVD	R11, ret+40(FP)
	RET
