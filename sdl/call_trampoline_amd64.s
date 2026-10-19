//go:build darwin || linux

#include "textflag.h"

// trampoline is entered from runtime.asmcgocall on the system stack with the
// System V AMD64 calling convention and DI = *callFrame (see
// call_trampoline.go and call_regclass.go for the layout). It keeps the frame
// pointer in the callee-saved BX and the entry stack pointer in the
// callee-saved R12, loads six integer argument registers, passes the seventh
// and eighth integer words on the 16-byte aligned stack, loads the eight
// floating-point argument registers, calls fn and stores AX and X0 back into
// the frame. Unused registers carry zeros, which a C function ignores.
TEXT trampoline<>(SB), NOSPLIT|NOFRAME, $0
	PUSHQ	BX
	PUSHQ	R12
	MOVQ	DI, BX
	MOVQ	SP, R12
	SUBQ	$16, SP
	ANDQ	$~15, SP

	MOVQ	56(BX), AX
	MOVQ	AX, 0(SP)
	MOVQ	64(BX), AX
	MOVQ	AX, 8(SP)

	MOVSD	72(BX), X0
	MOVSD	80(BX), X1
	MOVSD	88(BX), X2
	MOVSD	96(BX), X3
	MOVSD	104(BX), X4
	MOVSD	112(BX), X5
	MOVSD	120(BX), X6
	MOVSD	128(BX), X7

	MOVQ	8(BX), DI
	MOVQ	16(BX), SI
	MOVQ	24(BX), DX
	MOVQ	32(BX), CX
	MOVQ	40(BX), R8
	MOVQ	48(BX), R9

	// AL bounds the vector registers used by a variadic callee; a
	// non-variadic callee ignores it.
	MOVL	$8, AX
	MOVQ	0(BX), R10
	CALL	R10

	MOVQ	AX, 136(BX)
	MOVSD	X0, 144(BX)

	MOVQ	R12, SP
	POPQ	R12
	POPQ	BX
	RET

GLOBL ·trampolineABI0(SB), NOPTR|RODATA, $8
DATA ·trampolineABI0(SB)/8, $trampoline<>(SB)
