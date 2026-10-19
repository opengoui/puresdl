#include "textflag.h"

// trampoline is entered from runtime.asmcgocall on the system stack with the
// Microsoft x64 calling convention and CX = *callFrame, whose sixteen
// argument words are already in parameter order (see call_windows_amd64.go).
// It keeps the frame pointer in the callee-saved BX and the entry stack
// pointer in the callee-saved R12, reserves the 32-byte register home area
// plus twelve stack argument slots on a 16-byte aligned stack, copies words 4
// to 15 into the stack slots, loads words 0 to 3 into both RCX, RDX, R8, R9
// and XMM0 to XMM3 (the callee reads whichever its parameter type uses),
// calls fn and stores AX and X0 back into the frame.
TEXT trampoline<>(SB), NOSPLIT|NOFRAME, $0
	PUSHQ	BX
	PUSHQ	R12
	MOVQ	CX, BX
	MOVQ	SP, R12
	SUBQ	$128, SP
	ANDQ	$~15, SP

	MOVQ	40(BX), AX
	MOVQ	AX, 32(SP)
	MOVQ	48(BX), AX
	MOVQ	AX, 40(SP)
	MOVQ	56(BX), AX
	MOVQ	AX, 48(SP)
	MOVQ	64(BX), AX
	MOVQ	AX, 56(SP)
	MOVQ	72(BX), AX
	MOVQ	AX, 64(SP)
	MOVQ	80(BX), AX
	MOVQ	AX, 72(SP)
	MOVQ	88(BX), AX
	MOVQ	AX, 80(SP)
	MOVQ	96(BX), AX
	MOVQ	AX, 88(SP)
	MOVQ	104(BX), AX
	MOVQ	AX, 96(SP)
	MOVQ	112(BX), AX
	MOVQ	AX, 104(SP)
	MOVQ	120(BX), AX
	MOVQ	AX, 112(SP)
	MOVQ	128(BX), AX
	MOVQ	AX, 120(SP)

	MOVQ	8(BX), CX
	MOVQ	16(BX), DX
	MOVQ	24(BX), R8
	MOVQ	32(BX), R9
	MOVSD	8(BX), X0
	MOVSD	16(BX), X1
	MOVSD	24(BX), X2
	MOVSD	32(BX), X3

	MOVQ	0(BX), AX
	CALL	AX

	MOVQ	AX, 136(BX)
	MOVSD	X0, 144(BX)

	MOVQ	R12, SP
	POPQ	R12
	POPQ	BX
	RET

GLOBL ·trampolineABI0(SB), NOPTR|RODATA, $8
DATA ·trampolineABI0(SB)/8, $trampoline<>(SB)
