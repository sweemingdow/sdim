package client

import (
	"fmt"
	"strings"
	"testing"
)

func TestMsgSplit(t *testing.T) {
	str := "chatType receiveID `fdfdsf fdsfs  fdsf`"
	sli := strings.SplitN(str, " ", 3)
	content := sli[2][1 : len(sli[2])-1]
	fmt.Printf("f:%+v, conent:%s\n", sli, content)
}
