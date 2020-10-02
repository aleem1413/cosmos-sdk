package keeper_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"

	"github.com/cosmos/cosmos-sdk/simapp"
	store "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	clienttypes "github.com/cosmos/cosmos-sdk/x/ibc/02-client/types"
	ibctmtypes "github.com/cosmos/cosmos-sdk/x/ibc/07-tendermint/types"
	commitmenttypes "github.com/cosmos/cosmos-sdk/x/ibc/23-commitment/types"
	"github.com/cosmos/cosmos-sdk/x/upgrade/keeper"
	"github.com/cosmos/cosmos-sdk/x/upgrade/types"
)

type KeeperTestSuite struct {
	suite.Suite

	homeDir string
	app     *simapp.SimApp
	ctx     sdk.Context
}

func (s *KeeperTestSuite) SetupTest() {
	app := simapp.Setup(false)
	homeDir := filepath.Join(s.T().TempDir(), "x_upgrade_keeper_test")
	app.UpgradeKeeper = keeper.NewKeeper( // recreate keeper in order to use a custom home path
		make(map[int64]bool), app.GetKey(types.StoreKey), app.AppCodec(), homeDir,
	)
	s.T().Log("home dir:", homeDir)
	s.homeDir = homeDir
	s.app = app
	s.ctx = app.BaseApp.NewContext(false, tmproto.Header{
		Time:   time.Now(),
		Height: 10,
	})
}

func (s *KeeperTestSuite) TestReadUpgradeInfoFromDisk() {
	// require no error when the upgrade info file does not exist
	_, err := s.app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	s.Require().NoError(err)

	expected := store.UpgradeInfo{
		Name:   "test_upgrade",
		Height: 100,
	}

	// create an upgrade info file
	s.Require().NoError(s.app.UpgradeKeeper.DumpUpgradeInfoToDisk(expected.Height, expected.Name))

	ui, err := s.app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	s.Require().NoError(err)
	s.Require().Equal(expected, ui)
}

func (s *KeeperTestSuite) TestScheduleUpgrade() {
	clientState := &ibctmtypes.ClientState{ChainId: "gaiachain"}
	cs, err := clienttypes.PackClientState(clientState)
	s.Require().NoError(err)

	altClientState := &ibctmtypes.ClientState{ChainId: "ethermint"}
	altCs, err := clienttypes.PackClientState(altClientState)
	s.Require().NoError(err)

	consState := ibctmtypes.NewConsensusState(time.Now(), commitmenttypes.NewMerkleRoot([]byte("app_hash")), []byte("next_vals_hash"))
	consAny, err := clienttypes.PackConsensusState(consState)
	s.Require().NoError(err)

	cases := []struct {
		name    string
		plan    types.Plan
		setup   func()
		expPass bool
	}{
		{
			name: "successful time schedule",
			plan: types.Plan{
				Name: "all-good",
				Info: "some text here",
				Time: s.ctx.BlockTime().Add(time.Hour),
			},
			setup:   func() {},
			expPass: true,
		},
		{
			name: "successful height schedule",
			plan: types.Plan{
				Name:   "all-good",
				Info:   "some text here",
				Height: 123450000,
			},
			setup:   func() {},
			expPass: true,
		},
		{
			name: "successful ibc schedule",
			plan: types.Plan{
				Name:                "all-good",
				Info:                "some text here",
				Height:              123450000,
				UpgradedClientState: cs,
			},
			setup:   func() {},
			expPass: true,
		},
		{
			name: "successful overwrite",
			plan: types.Plan{
				Name:   "all-good",
				Info:   "some text here",
				Height: 123450000,
			},
			setup: func() {
				s.app.UpgradeKeeper.ScheduleUpgrade(s.ctx, types.Plan{
					Name:   "alt-good",
					Info:   "new text here",
					Height: 543210000,
				})
			},
			expPass: true,
		},
		{
			name: "successful IBC overwrite",
			plan: types.Plan{
				Name:                "all-good",
				Info:                "some text here",
				Height:              123450000,
				UpgradedClientState: cs,
			},
			setup: func() {
				s.app.UpgradeKeeper.ScheduleUpgrade(s.ctx, types.Plan{
					Name:                "alt-good",
					Info:                "new text here",
					Height:              543210000,
					UpgradedClientState: altCs,
				})
			},
			expPass: true,
		},
		{
			name: "successful IBC overwrite with non IBC plan",
			plan: types.Plan{
				Name:   "all-good",
				Info:   "some text here",
				Height: 123450000,
			},
			setup: func() {
				s.app.UpgradeKeeper.ScheduleUpgrade(s.ctx, types.Plan{
					Name:                "alt-good",
					Info:                "new text here",
					Height:              543210000,
					UpgradedClientState: altCs,
				})
			},
			expPass: true,
		},

		{
			name: "unsuccessful schedule: invalid plan",
			plan: types.Plan{
				Height: 123450000,
			},
			setup:   func() {},
			expPass: false,
		},
		{
			name: "unsuccessful time schedule: due date in past",
			plan: types.Plan{
				Name: "all-good",
				Info: "some text here",
				Time: s.ctx.BlockTime(),
			},
			setup:   func() {},
			expPass: false,
		},
		{
			name: "unsuccessful height schedule: due date in past",
			plan: types.Plan{
				Name:   "all-good",
				Info:   "some text here",
				Height: 1,
			},
			setup:   func() {},
			expPass: false,
		},
		{
			name: "unsuccessful schedule: schedule already executed",
			plan: types.Plan{
				Name:   "all-good",
				Info:   "some text here",
				Height: 123450000,
			},
			setup: func() {
				s.app.UpgradeKeeper.SetUpgradeHandler("all-good", func(_ sdk.Context, _ types.Plan) {})
				s.app.UpgradeKeeper.ApplyUpgrade(s.ctx, types.Plan{
					Name:   "all-good",
					Info:   "some text here",
					Height: 123450000,
				})
			},
			expPass: false,
		},
		{
			name: "unsuccessful IBC schedule: UpgradedClientState is not valid client state",
			plan: types.Plan{
				Name:                "all-good",
				Info:                "some text here",
				Height:              123450000,
				UpgradedClientState: consAny,
			},
			setup:   func() {},
			expPass: false,
		},
	}

	for _, tc := range cases {
		tc := tc

		s.Run(tc.name, func() {
			// reset suite
			s.SetupTest()

			// setup test case
			tc.setup()

			err := s.app.UpgradeKeeper.ScheduleUpgrade(s.ctx, tc.plan)

			if tc.expPass {
				s.Require().NoError(err, "valid test case failed")
				if tc.plan.UpgradedClientState != nil {
					got, height, err := s.app.UpgradeKeeper.GetUpgradedClient(s.ctx)
					s.Require().NoError(err)
					s.Require().Equal(tc.plan.Height, height, "upgradedClient not stored at correct upgrade height")
					s.Require().Equal(clientState, got, "upgradedClient not equal to expected value")
				} else {
					// check that upgraded client is empty if latest plan does not specify an upgraded client
					got, _, err := s.app.UpgradeKeeper.GetUpgradedClient(s.ctx)
					s.Require().Error(err)
					s.Require().Nil(got)
				}
			} else {
				s.Require().Error(err, "invalid test case passed")
			}
		})
	}
}

func TestKeeperTestSuite(t *testing.T) {
	suite.Run(t, new(KeeperTestSuite))
}
