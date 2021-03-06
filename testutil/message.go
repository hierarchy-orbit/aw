package testutil

import (
	"math"
	"math/rand"

	"github.com/renproject/aw/protocol"
)

func InvalidMessageVersion() protocol.MessageVersion {
	version := protocol.V1
	for version == protocol.V1 {
		version = protocol.MessageVersion(rand.Intn(math.MaxUint16))
	}
	return version
}

func InvalidMessageVariant(validVariants ...protocol.MessageVariant) protocol.MessageVariant {
	variant := protocol.MessageVariant(rand.Intn(math.MaxUint16))
	valid := func(v protocol.MessageVariant) bool {
		if validVariants == nil {
			return protocol.ValidateMessageVariant(v) == nil
		}

		for _, validVariant := range validVariants {
			if validVariant == v {
				return true
			}
		}
		return false
	}
	for valid(variant) {
		variant = protocol.MessageVariant(rand.Intn(math.MaxUint16))
	}
	return variant
}

func RandomBytes(length int) []byte {
	slice := make([]byte, length)
	_, err := rand.Read(slice)
	if err != nil {
		panic(err)
	}
	return slice
}

func RandomMessageBody() protocol.MessageBody {
	length := rand.Intn(512)
	return RandomBytes(length)
}

func RandomMessageVariant() protocol.MessageVariant {
	allVariants := []protocol.MessageVariant{
		protocol.Ping,
		protocol.Pong,
		protocol.Cast,
		protocol.Multicast,
		protocol.Broadcast,
	}
	return allVariants[rand.Intn(len(allVariants))]
}

func RandomMessage(version protocol.MessageVersion, variant protocol.MessageVariant) protocol.Message {
	body := RandomMessageBody()
	groupID := protocol.NilGroupID
	length := 8
	if variant == protocol.Multicast || variant == protocol.Broadcast {
		groupID = RandomGroupID()
		length = 40
	}
	return protocol.Message{
		Length:  protocol.MessageLength(length + len(body)),
		Version: version,
		Variant: variant,
		GroupID: groupID,
		Body:    body,
	}
}
