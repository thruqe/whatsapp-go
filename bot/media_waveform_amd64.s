#include "textflag.h"

// func sumAbsPCM16(pcm []byte) uint64
TEXT ·sumAbsPCM16(SB), NOSPLIT, $0-32
	MOVQ pcm_base+0(FP), SI
	MOVQ pcm_len+8(FP), CX
	SHRQ $1, CX             // CX = number of int16 samples

	XORQ AX, AX             // scalar total

	PXOR X0, X0             // X0 = accumulator (2 x uint64)
	PXOR X7, X7             // X7 = 0

loop16:
	CMPQ CX, $16
	JL loop8

	// Process 8 samples from 0(SI)
	MOVOU 0(SI), X2
	PABSW X2, X2

	MOVO X2, X3
	PUNPCKLWL X7, X2        // X2 = 4 x uint32 (low)
	PUNPCKHWL X7, X3        // X3 = 4 x uint32 (high)

	MOVO X2, X4
	PUNPCKLLQ X7, X2        // X2 = 2 x uint64
	PUNPCKHLQ X7, X4        // X4 = 2 x uint64
	PADDQ X2, X0
	PADDQ X4, X0

	MOVO X3, X5
	PUNPCKLLQ X7, X3        // X3 = 2 x uint64
	PUNPCKHLQ X7, X5        // X5 = 2 x uint64
	PADDQ X3, X0
	PADDQ X5, X0

	// Process 8 samples from 16(SI)
	MOVOU 16(SI), X2
	PABSW X2, X2

	MOVO X2, X3
	PUNPCKLWL X7, X2
	PUNPCKHWL X7, X3

	MOVO X2, X4
	PUNPCKLLQ X7, X2
	PUNPCKHLQ X7, X4
	PADDQ X2, X0
	PADDQ X4, X0

	MOVO X3, X5
	PUNPCKLLQ X7, X3
	PUNPCKHLQ X7, X5
	PADDQ X3, X0
	PADDQ X5, X0

	ADDQ $32, SI
	SUBQ $16, CX
	JMP loop16

loop8:
	CMPQ CX, $8
	JL reduce

	MOVOU 0(SI), X2
	PABSW X2, X2

	MOVO X2, X3
	PUNPCKLWL X7, X2
	PUNPCKHWL X7, X3

	MOVO X2, X4
	PUNPCKLLQ X7, X2
	PUNPCKHLQ X7, X4
	PADDQ X2, X0
	PADDQ X4, X0

	MOVO X3, X5
	PUNPCKLLQ X7, X3
	PUNPCKHLQ X7, X5
	PADDQ X3, X0
	PADDQ X5, X0

	ADDQ $16, SI
	SUBQ $8, CX

reduce:
	// X0 has 2 x uint64: [high, low]
	MOVHLPS X0, X1          // X1 = [-, high]
	PADDQ X1, X0            // X0[0] = low + high
	MOVQ X0, AX

loop1:
	CMPQ CX, $0
	JLE done

	MOVWLZX 0(SI), BX       // load uint16
	MOVWQSX BX, R8          // sign-extend to int64
	TESTQ R8, R8
	JGE pos
	NEGQ R8
pos:
	ADDQ R8, AX

	ADDQ $2, SI
	DECQ CX
	JMP loop1

done:
	MOVQ AX, ret+24(FP)
	RET
