#ifndef _STUB_STDINT_H
#define _STUB_STDINT_H
typedef __INT8_TYPE__ int8_t; typedef __INT16_TYPE__ int16_t; typedef __INT32_TYPE__ int32_t; typedef __INT64_TYPE__ int64_t;
typedef __UINT8_TYPE__ uint8_t; typedef __UINT16_TYPE__ uint16_t; typedef __UINT32_TYPE__ uint32_t; typedef __UINT64_TYPE__ uint64_t;
typedef __INTPTR_TYPE__ intptr_t; typedef __UINTPTR_TYPE__ uintptr_t;
typedef __INTMAX_TYPE__ intmax_t; typedef __UINTMAX_TYPE__ uintmax_t;
#define INT8_C(c) __INT8_C(c)
#define INT16_C(c) __INT16_C(c)
#define INT32_C(c) __INT32_C(c)
#define INT64_C(c) __INT64_C(c)
#define UINT8_C(c) __UINT8_C(c)
#define UINT16_C(c) __UINT16_C(c)
#define UINT32_C(c) __UINT32_C(c)
#define UINT64_C(c) __UINT64_C(c)
#define INT8_MAX 127
#define INT8_MIN (-128)
#define UINT8_MAX 255
#define INT16_MAX 32767
#define INT16_MIN (-32768)
#define UINT16_MAX 65535
#define INT32_MAX 2147483647
#define INT32_MIN (-INT32_MAX-1)
#define UINT32_MAX 4294967295U
#define INT64_MAX 9223372036854775807LL
#define INT64_MIN (-INT64_MAX-1LL)
#define UINT64_MAX 18446744073709551615ULL
#define SIZE_MAX __SIZE_MAX__
#define INTPTR_MAX __INTPTR_MAX__
#define UINTPTR_MAX __UINTPTR_MAX__
#endif
