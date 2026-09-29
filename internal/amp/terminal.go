package amp

import "strings"

func stripTerminalSequences(input string) string {
	var output strings.Builder

	for index := 0; index < len(input); {
		if input[index] != 0x1b {
			output.WriteByte(input[index])
			index++
			continue
		}

		index++

		if index >= len(input) {
			break
		}

		switch input[index] {
		case '[':
			index++

			for index < len(input) {
				current := input[index]
				index++

				if current >= 0x40 &&
					current <= 0x7e {
					break
				}
			}

		case ']':
			index++

			for index < len(input) {
				if input[index] == 0x07 {
					index++
					break
				}

				if input[index] == 0x1b &&
					index+1 < len(input) &&
					input[index+1] == '\\' {
					index += 2
					break
				}

				index++
			}

		default:
			index++
		}
	}

	return output.String()
}
