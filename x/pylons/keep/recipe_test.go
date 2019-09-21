package keep

import (
	"reflect"
	"testing"

	"github.com/MikeSofaer/pylons/x/pylons/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestKeeperGetRecipe(t *testing.T) {
	mockedCoinInput := SetupTestCoinInput()

	sender, _ := sdk.AccAddressFromBech32("cosmos1y8vysg9hmvavkdxpvccv2ve3nssv5avm0kt337")

	mockedCoinInput.Bk.AddCoins(mockedCoinInput.Ctx, sender, types.PremiumTier.Fee)

	cases := map[string]struct {
		cookbookName string
		recipeName   string
		desc         string
		sender       sdk.AccAddress
		level        types.Level
		desiredError string
		showError    bool
	}{
		"basic flow test": {
			cookbookName: "cookbook-00001",
			recipeName:   "recipe-00001",
			desc:         "this has to meet character limits",
			sender:       sender,
			level:        1,
			desiredError: "",
			showError:    false,
		},
	}
	for testName, tc := range cases {
		t.Run(testName, func(t *testing.T) {
			cb := types.NewCookbook(
				"example@example.com", // msg.SupportEmail,
				tc.sender,             // msg.Sender,
				"1.0.0",               // msg.Version,
				tc.cookbookName,       // msg.Name,
				tc.desc,               // msg.Description,
				"SketchyCo",           // msg.Developer
			)
			err := mockedCoinInput.PlnK.SetCookbook(mockedCoinInput.Ctx, cb)
			require.True(t, err == nil)

			recipe := types.NewRecipe(tc.recipeName, cb.ID, tc.desc,
				types.GenCoinInputList("wood", 5),
				types.GenCoinOutputList("chair", 1),
				types.GenItemInputList("Raichu"),
				types.GenItemOutputList("Raichu"),
				0,
				tc.sender)
			mockedCoinInput.PlnK.SetRecipe(mockedCoinInput.Ctx, recipe)
			readRecipe, err2 := mockedCoinInput.PlnK.GetRecipe(mockedCoinInput.Ctx, recipe.ID)
			require.True(t, err2 == nil)

			require.True(t, err2 == nil)
			require.True(t, reflect.DeepEqual(recipe, readRecipe))
		})
	}
}