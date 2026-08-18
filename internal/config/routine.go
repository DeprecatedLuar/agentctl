package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/DeprecatedLuar/agentctl/internal/resolution"
	"github.com/teambition/rrule-go"
)

const (
	// Directory and file name
	routinesDir = "routines"

	// Reserved TOML keys (not parameters)
	keyRoutineCommand     = "command"
	keyRoutineDescription = "description"
	keyRoutineEnabled     = "enabled"
	keyRoutineSchedule    = "schedule"

	// schedule.* keys
	keyScheduleTime  = "time"
	keyScheduleEvery = "every"
	keyScheduleRRule = "rrule"

	// Time format for schedule.time
	scheduleTimeFormat = "15:04"

	minDayOfMonth = 1
	maxDayOfMonth = 31
)

// ScheduleMode identifies which single trigger shape a routine's
// schedule.every value was written in. Never mixed within one file.
type ScheduleMode int

const (
	ModeNone ScheduleMode = iota
	ModeWeekday
	ModeDayOfMonth
	ModeDayInterval
	ModeHourInterval
	ModeRRule
)

var weekdayNames = map[string]time.Weekday{
	"sun": time.Sunday,
	"mon": time.Monday,
	"tue": time.Tuesday,
	"wed": time.Wednesday,
	"thu": time.Thursday,
	"fri": time.Friday,
	"sat": time.Saturday,
}

var (
	dayIntervalPattern  = regexp.MustCompile(`^(\d+)d$`)
	hourIntervalPattern = regexp.MustCompile(`^(\d+)h$`)
	dayOfMonthPattern   = regexp.MustCompile(`^(\d+)(st|nd|rd|th)$`)
)

// Schedule is the compiled, validated form of a routine's [schedule] table.
type Schedule struct {
	Mode ScheduleMode

	// ModeWeekday / ModeDayOfMonth / ModeDayInterval / ModeHourInterval
	Time        time.Time      // Wall-clock time-of-day (ModeWeekday/DayOfMonth/DayInterval), zero for ModeHourInterval
	Weekdays    []time.Weekday // ModeWeekday
	DaysOfMonth []int          // ModeDayOfMonth
	Interval    time.Duration  // ModeDayInterval / ModeHourInterval

	// ModeRRule
	RRuleSet *rrule.Set
}

type RoutineConfig struct {
	Name        string
	Command     string `toml:"command"`
	Description string `toml:"description"`
	Enabled     bool
	Schedule    Schedule
	Parameters  map[string]Parameter
	Path        string // Absolute path to the .toml file
}

// LoadRoutines discovers and parses all routines/*.toml files under agentPath.
// Mirrors LoadTools: recursive discovery, one ValidationIssue slice for the
// whole set, malformed routines produce IssueError (which the caller treats
// as blocking startup, same as any other config error).
func LoadRoutines(agentPath string) ([]RoutineConfig, []ValidationIssue) {
	var issues []ValidationIssue
	var routines []RoutineConfig

	routinesPath := filepath.Join(agentPath, routinesDir)

	if _, err := os.Stat(routinesPath); err != nil {
		// No routines/ directory - not an error, subsystem is simply inert.
		return nil, nil
	}

	var filesToLoad []string
	if err := walkToolsDir(routinesPath, make(map[string]bool), &filesToLoad); err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("routines/: %v", err),
		})
		return nil, issues
	}

	for _, routinePath := range filesToLoad {
		routine, routineIssues := loadRoutine(routinePath, routinesPath)
		issues = append(issues, routineIssues...)

		hasError := false
		for _, issue := range routineIssues {
			if issue.Type == IssueError {
				hasError = true
				break
			}
		}
		if !hasError {
			routines = append(routines, routine)
		}
	}

	return routines, issues
}

// LoadRoutine (re-)parses a single routine file. Used for the scheduler's
// pre-fire freshness gate: re-read the specific routine immediately before
// executing it, to catch a delete/disable/edit that landed after the last
// fsnotify-driven reload but before the fire moment.
func LoadRoutine(path string, routinesBasePath string) (RoutineConfig, []ValidationIssue) {
	return loadRoutine(path, routinesBasePath)
}

func loadRoutine(path string, routinesBasePath string) (RoutineConfig, []ValidationIssue) {
	var issues []ValidationIssue

	filename := filepath.Base(path)
	relPath := strings.TrimPrefix(path, routinesBasePath)
	relPath = strings.TrimPrefix(relPath, string(filepath.Separator))
	if relPath == "" {
		relPath = filename
	}
	errPrefix := fmt.Sprintf("routines/%s", relPath)

	data, err := os.ReadFile(path)
	if err != nil {
		issues = append(issues, ValidationIssue{Type: IssueError, Message: fmt.Sprintf("%s: %v", errPrefix, err)})
		return RoutineConfig{}, issues
	}

	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		issues = append(issues, ValidationIssue{Type: IssueError, Message: fmt.Sprintf("%s: failed to parse TOML: %v", errPrefix, err)})
		return RoutineConfig{}, issues
	}

	routine := RoutineConfig{
		Name:       strings.TrimSuffix(filename, tomlExtension),
		Path:       path,
		Enabled:    true, // default
		Parameters: make(map[string]Parameter),
	}

	if cmd, ok := raw[keyRoutineCommand].(string); ok {
		routine.Command = cmd
	}
	if desc, ok := raw[keyRoutineDescription].(string); ok {
		routine.Description = desc
	}
	if enabled, ok := raw[keyRoutineEnabled].(bool); ok {
		routine.Enabled = enabled
	}

	if routine.Command == "" {
		issues = append(issues, ValidationIssue{Type: IssueError, Message: fmt.Sprintf("%s: command field is required", errPrefix)})
	}

	scheduleRaw, ok := raw[keyRoutineSchedule].(map[string]interface{})
	if !ok {
		issues = append(issues, ValidationIssue{Type: IssueError, Message: fmt.Sprintf("%s: [schedule] table is required", errPrefix)})
	} else {
		schedule, scheduleIssues := parseSchedule(scheduleRaw, errPrefix)
		issues = append(issues, scheduleIssues...)
		routine.Schedule = schedule
	}

	// Remaining top-level tables are parameters (same mechanism as tools).
	for key, value := range raw {
		switch key {
		case keyRoutineCommand, keyRoutineDescription, keyRoutineEnabled, keyRoutineSchedule:
			continue
		}

		paramMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		param := Parameter{
			Type:    defaultParameterType,
			Enabled: true,
		}

		if typ, ok := paramMap["type"].(string); ok {
			param.Type = typ
		}
		if req, ok := paramMap["required"].(bool); ok {
			param.Required = req
		}
		if enabled, ok := paramMap["enabled"].(bool); ok {
			param.Enabled = enabled
		}
		if ret, ok := paramMap["return"].(string); ok {
			param.Return = ret
			if ret != "" {
				if err := resolution.ValidateSyntax(ret); err != nil {
					issues = append(issues, ValidationIssue{
						Type:    IssueError,
						Message: fmt.Sprintf("%s: parameter '%s' return field: %v", errPrefix, key, err),
					})
				}
			}
		}
		if desc, ok := paramMap["description"].(string); ok {
			param.Description = desc
		}

		routine.Parameters[key] = param
	}

	return routine, issues
}

// parseSchedule validates and compiles a routine's [schedule] table.
// Exactly one of rrule, or (time + every), determines the mode - see the
// mode-detection table in implementation-plan.md. Any ambiguity or conflict
// is a hard error, never silently resolved.
func parseSchedule(raw map[string]interface{}, errPrefix string) (Schedule, []ValidationIssue) {
	var issues []ValidationIssue

	timeStr, _ := raw[keyScheduleTime].(string)
	everyStr, _ := raw[keyScheduleEvery].(string)
	rruleStr, _ := raw[keyScheduleRRule].(string)

	hasTime := timeStr != ""
	hasEvery := everyStr != ""
	hasRRule := rruleStr != ""

	if hasRRule && (hasTime || hasEvery) {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("%s: schedule.rrule is mutually exclusive with schedule.time/schedule.every", errPrefix),
		})
		return Schedule{}, issues
	}

	if hasRRule {
		set, err := rrule.StrToRRuleSet(rruleStr)
		if err != nil {
			issues = append(issues, ValidationIssue{
				Type:    IssueError,
				Message: fmt.Sprintf("%s: schedule.rrule: invalid RRULE: %v", errPrefix, err),
			})
			return Schedule{}, issues
		}
		return Schedule{Mode: ModeRRule, RRuleSet: set}, issues
	}

	if !hasEvery {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("%s: [schedule] requires either rrule, or every (with time)", errPrefix),
		})
		return Schedule{}, issues
	}

	mode, weekdays, daysOfMonth, interval, err := classifyEvery(everyStr)
	if err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("%s: schedule.every: %v", errPrefix, err),
		})
		return Schedule{}, issues
	}

	schedule := Schedule{
		Mode:        mode,
		Weekdays:    weekdays,
		DaysOfMonth: daysOfMonth,
		Interval:    interval,
	}

	switch mode {
	case ModeHourInterval:
		if hasTime {
			issues = append(issues, ValidationIssue{
				Type:    IssueError,
				Message: fmt.Sprintf("%s: schedule.time is not allowed with an hour-interval schedule.every (%q)", errPrefix, everyStr),
			})
			return Schedule{}, issues
		}
	default: // ModeWeekday, ModeDayOfMonth, ModeDayInterval
		if !hasTime {
			issues = append(issues, ValidationIssue{
				Type:    IssueError,
				Message: fmt.Sprintf("%s: schedule.time is required for schedule.every %q", errPrefix, everyStr),
			})
			return Schedule{}, issues
		}
		parsedTime, err := time.Parse(scheduleTimeFormat, timeStr)
		if err != nil {
			issues = append(issues, ValidationIssue{
				Type:    IssueError,
				Message: fmt.Sprintf("%s: schedule.time %q must be HH:MM: %v", errPrefix, timeStr, err),
			})
			return Schedule{}, issues
		}
		schedule.Time = parsedTime
	}

	return schedule, issues
}

// classifyEvery determines schedule.every's mode from its shape alone and
// parses it accordingly. Every token must belong to the same category -
// mixing (e.g. "mon,3d") is a hard error, matching cron's OR-ambiguity
// footgun this design deliberately avoids.
func classifyEvery(everyStr string) (ScheduleMode, []time.Weekday, []int, time.Duration, error) {
	tokens := strings.Split(everyStr, ",")
	for i := range tokens {
		tokens[i] = strings.TrimSpace(strings.ToLower(tokens[i]))
	}

	if len(tokens) == 1 {
		if m := dayIntervalPattern.FindStringSubmatch(tokens[0]); m != nil {
			n, _ := strconv.Atoi(m[1])
			return ModeDayInterval, nil, nil, time.Duration(n) * 24 * time.Hour, nil
		}
		if m := hourIntervalPattern.FindStringSubmatch(tokens[0]); m != nil {
			n, _ := strconv.Atoi(m[1])
			return ModeHourInterval, nil, nil, time.Duration(n) * time.Hour, nil
		}
	}

	// Try weekday-list: every token must be a known weekday abbreviation.
	if allWeekdays(tokens) {
		weekdays := make([]time.Weekday, 0, len(tokens))
		for _, tok := range tokens {
			weekdays = append(weekdays, weekdayNames[tok])
		}
		return ModeWeekday, weekdays, nil, 0, nil
	}

	// Try day-of-month list: every token must be an ordinal such as "1st".
	if allDayOfMonthOrdinals(tokens) {
		days := make([]int, 0, len(tokens))
		for _, tok := range tokens {
			n, err := parseDayOfMonthOrdinal(tok)
			if err != nil {
				return ModeNone, nil, nil, 0, err
			}
			days = append(days, n)
		}
		return ModeDayOfMonth, nil, days, 0, nil
	}
	if allIntegers(tokens) {
		return ModeNone, nil, nil, 0, fmt.Errorf("bare day-of-month numbers are not supported; use ordinals such as \"1st,15th\"")
	}

	return ModeNone, nil, nil, 0, fmt.Errorf("unrecognized or mixed-type value %q (expected a weekday list, an ordinal day-of-month list, \"Nd\", or \"Nh\" - never mixed)", everyStr)
}

func allWeekdays(tokens []string) bool {
	for _, tok := range tokens {
		if _, ok := weekdayNames[tok]; !ok {
			return false
		}
	}
	return true
}

func allIntegers(tokens []string) bool {
	for _, tok := range tokens {
		if _, err := strconv.Atoi(tok); err != nil {
			return false
		}
	}
	return true
}

func allDayOfMonthOrdinals(tokens []string) bool {
	for _, tok := range tokens {
		if !dayOfMonthPattern.MatchString(tok) {
			return false
		}
	}
	return true
}

func parseDayOfMonthOrdinal(token string) (int, error) {
	match := dayOfMonthPattern.FindStringSubmatch(token)
	if match == nil {
		return 0, fmt.Errorf("invalid day-of-month ordinal %q", token)
	}

	day, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf("invalid day-of-month ordinal %q: %w", token, err)
	}
	if day < minDayOfMonth || day > maxDayOfMonth {
		return 0, fmt.Errorf("day-of-month value %d out of range %d-%d", day, minDayOfMonth, maxDayOfMonth)
	}

	expectedSuffix := ordinalSuffix(day)
	if match[2] != expectedSuffix {
		return 0, fmt.Errorf("day-of-month ordinal %q has the wrong suffix; expected %q", token, strconv.Itoa(day)+expectedSuffix)
	}
	return day, nil
}

func ordinalSuffix(day int) string {
	if day >= 11 && day <= 13 {
		return "th"
	}
	switch day % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	default:
		return "th"
	}
}
