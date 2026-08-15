package converter

import "strings"

type selectClauses struct {
	columns string
	from    string
	where   string
	groupBy string
	having  string
	orderBy string
	limit   string
}

type selectClauseMarker struct {
	name  string
	start int
	end   int
}

func splitTopLevelSelectClauses(sql string) selectClauses {
	markers := findTopLevelSelectClauseMarkers(sql)
	result := selectClauses{columns: strings.TrimSpace(sql)}
	if len(markers) == 0 {
		return result
	}

	result.columns = strings.TrimSpace(sql[:markers[0].start])
	for i, marker := range markers {
		valueEnd := len(sql)
		if i+1 < len(markers) {
			valueEnd = markers[i+1].start
		}
		value := strings.TrimSpace(strings.TrimSuffix(sql[marker.end:valueEnd], ";"))
		switch marker.name {
		case "FROM":
			result.from = value
		case "WHERE":
			result.where = value
		case "GROUP BY":
			result.groupBy = value
		case "HAVING":
			result.having = value
		case "ORDER BY":
			result.orderBy = value
		case "LIMIT":
			result.limit = value
		}
	}

	return result
}

func findTopLevelSelectClauseMarkers(sql string) []selectClauseMarker {
	markers := []selectClauseMarker{}
	depth := 0
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false

	for index := 0; index < len(sql); index++ {
		character := sql[index]
		switch character {
		case '\'':
			if !inDoubleQuote && !inBacktick {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote && !inBacktick {
				inDoubleQuote = !inDoubleQuote
			}
		case '`':
			if !inSingleQuote && !inDoubleQuote {
				inBacktick = !inBacktick
			}
		case '(':
			if !inSingleQuote && !inDoubleQuote && !inBacktick {
				depth++
			}
		case ')':
			if !inSingleQuote && !inDoubleQuote && !inBacktick && depth > 0 {
				depth--
			}
		}
		if depth != 0 || inSingleQuote || inDoubleQuote || inBacktick {
			continue
		}

		for _, keyword := range []string{"GROUP BY", "ORDER BY", "FROM", "WHERE", "HAVING", "LIMIT"} {
			if hasKeywordAt(sql, index, keyword) {
				markers = append(markers, selectClauseMarker{
					name:  keyword,
					start: index,
					end:   index + len(keyword),
				})
				index += len(keyword) - 1
				break
			}
		}
	}

	return markers
}

func hasKeywordAt(sql string, start int, keyword string) bool {
	end := start + len(keyword)
	if end > len(sql) || !strings.EqualFold(sql[start:end], keyword) {
		return false
	}
	if start > 0 && isIdentifierCharacter(sql[start-1]) {
		return false
	}
	return end == len(sql) || !isIdentifierCharacter(sql[end])
}

func isIdentifierCharacter(character byte) bool {
	return character == '_' || character >= '0' && character <= '9' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}
