package handler_test

import (
	feedomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/fee"
	rewarddomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/reward"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
)

// newFeeService and newRewardService keep every handler test's Config wiring
// identical without repeating the domain imports in each test file.

func newFeeService(db repository.Database) feedomain.Service {
	return feedomain.New(db)
}

func newRewardService(db repository.Database) rewarddomain.Service {
	return rewarddomain.New(db)
}
