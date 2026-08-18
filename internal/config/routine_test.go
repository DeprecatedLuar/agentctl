package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeRoutine(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create routine dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write routine: %v", err)
	}
	return path
}

func firstError(issues []ValidationIssue) string {
	for _, issue := range issues {
		if issue.Type == IssueError {
			return issue.Message
		}
	}
	return ""
}

func TestLoadRoutines_NoDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	routines, issues := LoadRoutines(tmpDir)
	if len(routines) != 0 {
		t.Errorf("expected 0 routines, got %d", len(routines))
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues when routines/ absent, got %v", issues)
	}
}

func TestLoadRoutines_EveryModes(t *testing.T) {
	tests := []struct {
		name         string
		schedule     string
		expectMode   ScheduleMode
		expectWeekly []time.Weekday
		expectDays   []int
		expectInterv time.Duration
	}{
		{
			name:         "weekday single",
			schedule:     `schedule.time = "14:00"` + "\n" + `schedule.every = "tue"`,
			expectMode:   ModeWeekday,
			expectWeekly: []time.Weekday{time.Tuesday},
		},
		{
			name:         "weekday list",
			schedule:     `schedule.time = "09:30"` + "\n" + `schedule.every = "mon,wed,fri"`,
			expectMode:   ModeWeekday,
			expectWeekly: []time.Weekday{time.Monday, time.Wednesday, time.Friday},
		},
		{
			name:       "day of month list",
			schedule:   `schedule.time = "08:00"` + "\n" + `schedule.every = "1st,2nd,3rd,4th,11th,12th,13th,21st,22nd,23rd,31st"`,
			expectMode: ModeDayOfMonth,
			expectDays: []int{1, 2, 3, 4, 11, 12, 13, 21, 22, 23, 31},
		},
		{
			name:         "day interval",
			schedule:     `schedule.time = "00:00"` + "\n" + `schedule.every = "3d"`,
			expectMode:   ModeDayInterval,
			expectInterv: 3 * 24 * time.Hour,
		},
		{
			name:         "hour interval",
			schedule:     `schedule.every = "6h"`,
			expectMode:   ModeHourInterval,
			expectInterv: 6 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			content := "command = \"echo hi\"\ndescription = \"test\"\n" + tt.schedule + "\n"
			writeRoutine(t, filepath.Join(tmpDir, routinesDir), "r.toml", content)

			routines, issues := LoadRoutines(tmpDir)
			if err := firstError(issues); err != "" {
				t.Fatalf("unexpected error: %s", err)
			}
			if len(routines) != 1 {
				t.Fatalf("expected 1 routine, got %d", len(routines))
			}

			sched := routines[0].Schedule
			if sched.Mode != tt.expectMode {
				t.Errorf("expected mode %v, got %v", tt.expectMode, sched.Mode)
			}
			if tt.expectWeekly != nil {
				if len(sched.Weekdays) != len(tt.expectWeekly) {
					t.Fatalf("expected %d weekdays, got %d", len(tt.expectWeekly), len(sched.Weekdays))
				}
				for i, w := range tt.expectWeekly {
					if sched.Weekdays[i] != w {
						t.Errorf("weekday[%d]: expected %v, got %v", i, w, sched.Weekdays[i])
					}
				}
			}
			if tt.expectDays != nil {
				if len(sched.DaysOfMonth) != len(tt.expectDays) {
					t.Fatalf("expected %d days, got %d", len(tt.expectDays), len(sched.DaysOfMonth))
				}
				for i, d := range tt.expectDays {
					if sched.DaysOfMonth[i] != d {
						t.Errorf("day[%d]: expected %d, got %d", i, d, sched.DaysOfMonth[i])
					}
				}
			}
			if tt.expectInterv != 0 && sched.Interval != tt.expectInterv {
				t.Errorf("expected interval %v, got %v", tt.expectInterv, sched.Interval)
			}
		})
	}
}

func TestLoadRoutines_RejectionCases(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectedSub string
	}{
		{
			name:        "missing schedule table",
			content:     `command = "echo hi"` + "\n",
			expectedSub: "[schedule] table is required",
		},
		{
			name:        "missing command",
			content:     "schedule.every = \"tue\"\nschedule.time = \"10:00\"\n",
			expectedSub: "command field is required",
		},
		{
			name:        "every missing without rrule",
			content:     `command = "echo hi"` + "\n" + `[schedule]` + "\n",
			expectedSub: "requires either rrule, or every",
		},
		{
			name:        "mixed weekday and interval",
			content:     "command = \"echo hi\"\nschedule.time = \"10:00\"\nschedule.every = \"mon,3d\"\n",
			expectedSub: "unrecognized or mixed-type value",
		},
		{
			name:        "mixed day-interval and hour-interval units",
			content:     "command = \"echo hi\"\nschedule.every = \"3d,6h\"\n",
			expectedSub: "unrecognized or mixed-type value",
		},
		{
			name:        "time missing for weekday mode",
			content:     "command = \"echo hi\"\nschedule.every = \"tue\"\n",
			expectedSub: "schedule.time is required",
		},
		{
			name:        "time missing for day-of-month mode",
			content:     "command = \"echo hi\"\nschedule.every = \"2nd,3rd\"\n",
			expectedSub: "schedule.time is required",
		},
		{
			name:        "time missing for day-interval mode",
			content:     "command = \"echo hi\"\nschedule.every = \"3d\"\n",
			expectedSub: "schedule.time is required",
		},
		{
			name:        "time forbidden with hour-interval",
			content:     "command = \"echo hi\"\nschedule.time = \"10:00\"\nschedule.every = \"6h\"\n",
			expectedSub: "schedule.time is not allowed with an hour-interval",
		},
		{
			name:        "rrule with time",
			content:     "command = \"echo hi\"\nschedule.time = \"10:00\"\nschedule.rrule = \"FREQ=DAILY\"\n",
			expectedSub: "mutually exclusive",
		},
		{
			name:        "rrule with every",
			content:     "command = \"echo hi\"\nschedule.every = \"tue\"\nschedule.rrule = \"FREQ=DAILY\"\n",
			expectedSub: "mutually exclusive",
		},
		{
			name:        "invalid time format",
			content:     "command = \"echo hi\"\nschedule.time = \"2pm\"\nschedule.every = \"tue\"\n",
			expectedSub: "must be HH:MM",
		},
		{
			name:        "bare day of month rejected",
			content:     "command = \"echo hi\"\nschedule.time = \"10:00\"\nschedule.every = \"1,15\"\n",
			expectedSub: "bare day-of-month numbers are not supported",
		},
		{
			name:        "day of month out of range",
			content:     "command = \"echo hi\"\nschedule.time = \"10:00\"\nschedule.every = \"32nd\"\n",
			expectedSub: "out of range",
		},
		{
			name:        "day of month with wrong suffix",
			content:     "command = \"echo hi\"\nschedule.time = \"10:00\"\nschedule.every = \"11st\"\n",
			expectedSub: "wrong suffix",
		},
		{
			name:        "invalid rrule string",
			content:     "command = \"echo hi\"\nschedule.rrule = \"not a valid rrule\"\n",
			expectedSub: "invalid RRULE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			writeRoutine(t, filepath.Join(tmpDir, routinesDir), "r.toml", tt.content)

			_, issues := LoadRoutines(tmpDir)
			msg := firstError(issues)
			if msg == "" {
				t.Fatalf("expected an error containing %q, got none (issues: %v)", tt.expectedSub, issues)
			}
			if !strings.Contains(msg, tt.expectedSub) {
				t.Errorf("expected error containing %q, got %q", tt.expectedSub, msg)
			}
		})
	}
}

func TestLoadRoutines_ValidRRule(t *testing.T) {
	tmpDir := t.TempDir()
	content := "command = \"echo hi\"\n" +
		"schedule.rrule = \"DTSTART:20260101T090000Z\\nRRULE:FREQ=MONTHLY;BYDAY=MO;BYSETPOS=1\"\n"
	writeRoutine(t, filepath.Join(tmpDir, routinesDir), "r.toml", content)

	routines, issues := LoadRoutines(tmpDir)
	if err := firstError(issues); err != "" {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(routines) != 1 {
		t.Fatalf("expected 1 routine, got %d", len(routines))
	}
	if routines[0].Schedule.Mode != ModeRRule {
		t.Errorf("expected ModeRRule, got %v", routines[0].Schedule.Mode)
	}
	if routines[0].Schedule.RRuleSet == nil {
		t.Error("expected RRuleSet to be compiled")
	}
}

func TestLoadRoutines_EnabledDefaultAndOverride(t *testing.T) {
	tmpDir := t.TempDir()
	content := "command = \"echo hi\"\nschedule.every = \"tue\"\nschedule.time = \"10:00\"\nenabled = false\n"
	writeRoutine(t, filepath.Join(tmpDir, routinesDir), "r.toml", content)

	routines, issues := LoadRoutines(tmpDir)
	if err := firstError(issues); err != "" {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(routines) != 1 {
		t.Fatalf("expected 1 routine, got %d", len(routines))
	}
	if routines[0].Enabled {
		t.Error("expected routine to be disabled")
	}
}

func TestLoadRoutines_ParamsWithReturn(t *testing.T) {
	tmpDir := t.TempDir()
	content := `command = "agentctl -a {{name}}"
schedule.every = "tue"
schedule.time = "10:00"

[name]
return = "{{$agentpath}}"
`
	writeRoutine(t, filepath.Join(tmpDir, routinesDir), "r.toml", content)

	routines, issues := LoadRoutines(tmpDir)
	if err := firstError(issues); err != "" {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(routines) != 1 {
		t.Fatalf("expected 1 routine, got %d", len(routines))
	}
	param, ok := routines[0].Parameters["name"]
	if !ok {
		t.Fatal("expected 'name' parameter to be present")
	}
	if param.Return != "{{$agentpath}}" {
		t.Errorf("expected return '{{$agentpath}}', got %q", param.Return)
	}
}
