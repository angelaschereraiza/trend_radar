package validation

import (
	"errors"
	"fmt"
	"time"
)

type Role string

const (
	Development Role = "development"
	WalkForward Role = "walk_forward"
	Holdout     Role = "holdout"
	Forward     Role = "forward"
)

type Window struct {
	TrainStart time.Time
	TrainEnd   time.Time
	TestStart  time.Time
	TestEnd    time.Time
}

func WalkForwardWindows(start, end time.Time, trainingYears, testYears, stepYears int) ([]Window, error) {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return nil, errors.New("valid chronological start and end dates are required")
	}
	if trainingYears < 1 || testYears < 1 || stepYears < 1 {
		return nil, errors.New("training, test and step years must be positive")
	}
	var windows []Window
	trainStart := start
	for {
		trainEnd := trainStart.AddDate(trainingYears, 0, 0).AddDate(0, 0, -1)
		testStart := trainEnd.AddDate(0, 0, 1)
		testEnd := testStart.AddDate(testYears, 0, 0).AddDate(0, 0, -1)
		if testEnd.After(end) {
			break
		}
		if !testStart.After(trainEnd) {
			return nil, fmt.Errorf("walk-forward window overlaps train and test")
		}
		windows = append(windows, Window{
			TrainStart: trainStart, TrainEnd: trainEnd,
			TestStart: testStart, TestEnd: testEnd,
		})
		trainStart = trainStart.AddDate(stepYears, 0, 0)
	}
	return windows, nil
}

func DatasetRoleForDate(date, developmentStart, developmentEnd, holdoutStart, holdoutEnd time.Time) (Role, error) {
	if date.IsZero() {
		return "", errors.New("date is required")
	}
	switch {
	case !date.Before(developmentStart) && !date.After(developmentEnd):
		return Development, nil
	case !date.Before(holdoutStart) && !date.After(holdoutEnd):
		return Holdout, nil
	default:
		return "", fmt.Errorf("date %s is outside configured development and holdout periods", date.Format("2006-01-02"))
	}
}
