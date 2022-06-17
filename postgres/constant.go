package postgres

const (
	conTypeIsNotNull  = 1
	conTypeDefault    = 2
	conTypePrimaryKey = 5
	conTypeForeignKey = 8
)

var SQLValueFunctionOpToConstant = map[string]string{
	"0": "CURRENT_DATE",
	"1": "CURRENT_TIME",
	"2": "CURRENT_TIME(%s)",
	"3": "CURRENT_TIMESTAMP",
	"4": "CURRENT_TIMESTAMP(%s)",
	"5": "LOCALTIME",
	"6": "LOCALTIME(%s)",
	"7": "LOCALTIMESTAMP",
	"8": "LOCALTIMESTAMP(%s)",
}
