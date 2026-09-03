//go:build amd64 && !purego

#include "textflag.h"

// func maxByteAVX2(d []byte) byte
// Extreme Byte Search (maximum): 32-byte VPMAXUB accumulation, scalar tail, then
// one scalar reduction over the 32-lane accumulator.
TEXT ·maxByteAVX2(SB), NOSPLIT, $32-25
	MOVQ d_base+0(FP), SI
	MOVQ d_len+8(FP), CX
	XORL AX, AX
	VPXOR Y0, Y0, Y0
	MOVQ CX, DX
	SHRQ $5, DX
	JZ   maxRem

maxBlk:
	VMOVDQU (SI), Y1
	VPMAXUB Y1, Y0, Y0
	ADDQ $32, SI
	DECQ DX
	JNZ  maxBlk

maxRem:
	ANDQ $31, CX
	JZ   maxReduce

maxTail:
	MOVBLZX (SI), BX
	CMPB BL, AL
	JBE  maxTailNext
	MOVL BX, AX

maxTailNext:
	INCQ SI
	DECQ CX
	JNZ  maxTail

maxReduce:
	VMOVDQU Y0, 0(SP)
	VZEROUPPER
	LEAQ 0(SP), SI
	MOVL $32, CX

maxRl:
	MOVBLZX (SI), BX
	CMPB BL, AL
	JBE  maxRlNext
	MOVL BX, AX

maxRlNext:
	INCQ SI
	DECQ CX
	JNZ  maxRl

	MOVB AL, ret+24(FP)
	RET

// func minByteAVX2(d []byte) byte
// Extreme Byte Search (minimum). Mirror of maxByteAVX2 with VPMINUB and an
// accumulator seeded to 0xFF.
TEXT ·minByteAVX2(SB), NOSPLIT, $32-25
	MOVQ d_base+0(FP), SI
	MOVQ d_len+8(FP), CX
	MOVL $0xFF, AX
	TESTQ CX, CX
	JZ   minEmpty
	VPCMPEQD Y0, Y0, Y0
	MOVQ CX, DX
	SHRQ $5, DX
	JZ   minRem

minBlk:
	VMOVDQU (SI), Y1
	VPMINUB Y1, Y0, Y0
	ADDQ $32, SI
	DECQ DX
	JNZ  minBlk

minRem:
	ANDQ $31, CX
	JZ   minReduce

minTail:
	MOVBLZX (SI), BX
	CMPB BL, AL
	JAE  minTailNext
	MOVL BX, AX

minTailNext:
	INCQ SI
	DECQ CX
	JNZ  minTail

minReduce:
	VMOVDQU Y0, 0(SP)
	VZEROUPPER
	LEAQ 0(SP), SI
	MOVL $32, CX

minRl:
	MOVBLZX (SI), BX
	CMPB BL, AL
	JAE  minRlNext
	MOVL BX, AX

minRlNext:
	INCQ SI
	DECQ CX
	JNZ  minRl

	MOVB AL, ret+24(FP)
	RET

minEmpty:
	MOVB $0, ret+24(FP)
	RET

// func indexCmpAVX2(d []byte, target byte, op uint64) int
// Range Scan. op selects the comparator: 0=GT, 1=GE, 2=LT, 3=LE. Unsigned
// compares are built from packed min/max + equality, so no 0x80 bias is needed:
//   x <= t  <=>  min(x,t) == x        x >= t  <=>  max(x,t) == x
//   x >  t  <=>  ~(x <= t)            x <  t  <=>  ~(x >= t)
TEXT ·indexCmpAVX2(SB), NOSPLIT, $0-48
	MOVQ d_base+0(FP), SI
	MOVQ d_len+8(FP), CX
	MOVBLZX target+24(FP), DX
	MOVQ op+32(FP), R8
	XORQ DI, DI
	MOVQ DX, X2
	VPBROADCASTB X2, Y2
	VPCMPEQD Y5, Y5, Y5
	MOVQ CX, R9
	SHRQ $5, R9
	JZ   icTail

icBlk:
	VMOVDQU (SI)(DI*1), Y0
	VPMINUB Y0, Y2, Y3
	VPMAXUB Y0, Y2, Y4
	VPCMPEQB Y0, Y3, Y3          // Y3 = LE mask
	VPCMPEQB Y0, Y4, Y4          // Y4 = GE mask
	CMPQ R8, $0
	JEQ  icGT
	CMPQ R8, $1
	JEQ  icGE
	CMPQ R8, $2
	JEQ  icLT
	VMOVDQU Y3, Y1               // LE
	JMP  icMask

icGT:
	VPANDN Y5, Y3, Y1           // ~LE
	JMP  icMask

icGE:
	VMOVDQU Y4, Y1
	JMP  icMask

icLT:
	VPANDN Y5, Y4, Y1          // ~GE

icMask:
	VPMOVMSKB Y1, BX
	TESTL BX, BX
	JNZ  icFoundBlk
	ADDQ $32, DI
	DECQ R9
	JNZ  icBlk

icTail:
	ANDQ $31, CX
	JZ   icNotFound

icTloop:
	MOVBLZX (SI)(DI*1), BX
	CMPQ R8, $0
	JEQ  icTGT
	CMPQ R8, $1
	JEQ  icTGE
	CMPQ R8, $2
	JEQ  icTLT
	CMPL BX, DX
	JLE  icFoundTail
	JMP  icTnext

icTGT:
	CMPL BX, DX
	JGT  icFoundTail
	JMP  icTnext

icTGE:
	CMPL BX, DX
	JGE  icFoundTail
	JMP  icTnext

icTLT:
	CMPL BX, DX
	JLT  icFoundTail

icTnext:
	INCQ DI
	DECQ CX
	JNZ  icTloop

icNotFound:
	MOVQ d_len+8(FP), DI
	JMP  icRet

icFoundBlk:
	BSFL BX, BX
	ADDQ BX, DI

icFoundTail:
icRet:
	VZEROUPPER
	MOVQ DI, ret+40(FP)
	RET
