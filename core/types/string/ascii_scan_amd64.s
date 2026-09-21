//go:build amd64 && !purego

#include "textflag.h"

// Read full 32-byte blocks only; scalar tail never crosses the string boundary.
TEXT ·scanASCIIAVX2(SB), NOSPLIT, $0-17
 MOVQ s_base+0(FP), SI
 MOVQ s_len+8(FP), CX
 CMPQ CX, $32
 JB tail
loop:
 VMOVDQU (SI), Y0
 VPMOVMSKB Y0, AX
 TESTL AX, AX
 JNE vectorFalse
 ADDQ $32, SI
 SUBQ $32, CX
 CMPQ CX, $32
 JAE loop
 VZEROUPPER
tail:
 TESTQ CX, CX
 JE yes
 MOVBLZX (SI), AX
 TESTL $128, AX
 JNE no
 INCQ SI
 DECQ CX
 JMP tail
vectorFalse:
 VZEROUPPER
no:
 MOVB $0, ret+16(FP)
 RET
yes:
 MOVB $1, ret+16(FP)
 RET
