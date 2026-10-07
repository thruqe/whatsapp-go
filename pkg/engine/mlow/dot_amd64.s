#include "textflag.h"

// func dotProdF32(a, b []float32, l int) float32
TEXT ·dotProdF32(SB), NOSPLIT, $0-60
	MOVQ a_base+0(FP), SI   // SI = pointer to a
	MOVQ b_base+24(FP), DI  // DI = pointer to b
	MOVQ l+48(FP), CX       // CX = l

	XORPS X0, X0            // X0 = accumulator 1 (0.0)
	XORPS X1, X1            // X1 = accumulator 2 (0.0)

	TESTQ CX, CX
	JLE done

loop8:
	CMPQ CX, $8
	JL loop4

	MOVUPS 0(SI), X2
	MOVUPS 0(DI), X3
	MULPS X3, X2
	ADDPS X2, X0

	MOVUPS 16(SI), X4
	MOVUPS 16(DI), X5
	MULPS X5, X4
	ADDPS X4, X1

	ADDQ $32, SI
	ADDQ $32, DI
	SUBQ $8, CX
	JMP loop8

loop4:
	CMPQ CX, $4
	JL loop1

	MOVUPS 0(SI), X2
	MOVUPS 0(DI), X3
	MULPS X3, X2
	ADDPS X2, X0

	ADDQ $16, SI
	ADDQ $16, DI
	SUBQ $4, CX

loop1:
	// Accumulate parallel vector registers
	ADDPS X1, X0

	// Horizontal sum of the 4 float32 lanes in X0: [d, c, b, a]
	// MOVHLPS copies high 2 floats [d, c] into low 2 floats of X2
	MOVHLPS X0, X2
	ADDPS X2, X0            // X0 = [-, -, b+d, a+c]
	MOVSHDUP X0, X2         // X2 = [-, -, b+d, b+d]
	ADDSS X2, X0            // X0 = [-, -, -, (a+c)+(b+d)]

loop1_tail:
	TESTQ CX, CX
	JLE done

	MOVSS 0(SI), X2
	MOVSS 0(DI), X3
	MULSS X3, X2
	ADDSS X2, X0

	ADDQ $4, SI
	ADDQ $4, DI
	DECQ CX
	JMP loop1_tail

done:
	MOVSS X0, ret+56(FP)
	RET
