package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimestamp(t *testing.T) {
	tm := time.UnixMicro(1659682167_829_999_999)
	require.Equal(t, int64(1659682167_829), tm.Unix())

	tm = time.UnixMicro(1659682167_828_999_999)
	require.Equal(t, int64(1659682167_828), tm.Unix())

}
