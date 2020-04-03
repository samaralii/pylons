package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"

	"github.com/Pylons-tech/pylons/x/pylons/keep"
	"github.com/Pylons-tech/pylons/x/pylons/msgs"
	"github.com/Pylons-tech/pylons/x/pylons/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/interpreter/functions"

	celTypes "github.com/google/cel-go/common/types"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

type ExecProcess struct {
	ctx          sdk.Context
	keeper       keep.Keeper
	recipe       types.Recipe
	matchedItems []types.Item
	ec           types.CelEnvCollection
}

func Contains(arr []int, it int) bool {
	for _, a := range arr {
		if a == it {
			return true
		}
	}
	return false
}

func (p *ExecProcess) SetMatchedItemsFromExecMsg(msg msgs.MsgExecuteRecipe) error {

	var inputItems []types.Item
	keys := make(map[string]bool)

	for _, id := range msg.ItemIDs {
		if _, value := keys[id]; !value {
			keys[id] = true

			item, err := p.keeper.GetItem(p.ctx, id)
			if err != nil {
				return err
			}
			if !item.Sender.Equals(msg.Sender) {
				return errors.New("item owner is not same as sender")
			}

			inputItems = append(inputItems, item)
		} else {
			return errors.New("multiple use of same item as item inputs")
		}
	}

	// we validate and match items
	var matchedItems []types.Item
	var matches bool
	for _, itemInput := range p.recipe.ItemInputs {
		matches = false

		for _, item := range inputItems {
			if itemInput.Matches(item) && len(item.OwnerRecipeID) == 0 {
				matchedItems = append(matchedItems, item)
				matches = true
				break
			}
		}

		if !matches {
			return errors.New("the item inputs dont match any items provided")
		}
	}
	p.matchedItems = matchedItems
	return nil
}

func (p *ExecProcess) Run(sender sdk.AccAddress) ([]byte, error) {
	err := p.GenerateCelEnvVarFromInputItems()

	outputs, err := p.recipe.Outputs.Actualize(p.ec)
	if err != nil {
		return []byte{}, err
	}
	ersl, err := p.AddExecutedResult(sender, outputs)

	if err != nil {
		return []byte{}, err
	}

	outputSTR, err2 := json.Marshal(ersl)

	if err2 != nil {
		return []byte{}, err2
	}
	return outputSTR, nil
}

func (p *ExecProcess) AddExecutedResult(sender sdk.AccAddress, outputs []int) ([]ExecuteRecipeSerialize, sdk.Error) {
	var ersl []ExecuteRecipeSerialize
	var err error
	usedItemInputIndexes := []int{}
	for _, outputIndex := range outputs {
		if len(p.recipe.Entries) <= outputIndex || outputIndex < 0 {
			return ersl, sdk.ErrInternal(fmt.Sprintf("index out of range entries[%d] with length %d on output", outputIndex, len(p.recipe.Entries)))
		}
		output := p.recipe.Entries[outputIndex]

		switch output.(type) {
		case types.CoinOutput:
			coinOutput, _ := output.(types.CoinOutput)
			var coinAmount int64
			if len(coinOutput.Count) > 0 {
				val64, err := p.ec.EvalInt64(coinOutput.Count)
				if err != nil {
					return ersl, sdk.ErrInternal(err.Error())
				}
				coinAmount = val64
			} else {
				return ersl, sdk.ErrInternal("length of coin output program shouldn't be zero")
			}
			ocl := sdk.Coins{sdk.NewCoin(coinOutput.Coin, sdk.NewInt(coinAmount))}

			_, _, err := p.keeper.CoinKeeper.AddCoins(p.ctx, sender, ocl)
			if err != nil {
				return ersl, err
			}
			ersl = append(ersl, ExecuteRecipeSerialize{
				Type:   "COIN",
				Coin:   coinOutput.Coin,
				Amount: coinAmount,
			})
		case types.ItemOutput:
			itemOutput, _ := output.(types.ItemOutput)
			var outputItem *types.Item

			if itemOutput.ModifyItem.ItemInputRef == -1 {
				outputItem, err = itemOutput.Item(p.recipe.CookbookID, sender, p.ec)
				if err != nil {
					return ersl, sdk.ErrInternal(err.Error())
				}
			} else {
				// Collect itemInputRefs that are used on output
				usedItemInputIndexes = append(usedItemInputIndexes, itemOutput.ModifyItem.ItemInputRef)

				// Modify item according to ModifyParams section
				outputItem, err = p.UpdateItemFromModifyParams(p.matchedItems[itemOutput.ModifyItem.ItemInputRef], itemOutput.ModifyItem)
				if err != nil {
					return ersl, sdk.ErrInternal(err.Error())
				}
			}
			if err = p.keeper.SetItem(p.ctx, *outputItem); err != nil {
				return ersl, sdk.ErrInternal(err.Error())
			}
			ersl = append(ersl, ExecuteRecipeSerialize{
				Type:   "ITEM",
				ItemID: outputItem.ID,
			})
		default:
			return ersl, sdk.ErrInternal("no item nor coin type created")
		}
	}

	// Remove items which are not referenced on output
	for idx, ci := range p.matchedItems {
		if !Contains(usedItemInputIndexes, idx) {
			p.keeper.DeleteItem(p.ctx, ci.ID)
		}
	}
	return ersl, nil
}

func (p *ExecProcess) UpdateItemFromModifyParams(targetItem types.Item, toMod types.ModifyItemType) (*types.Item, sdk.Error) {

	if dblKeyValues, err := toMod.Doubles.Actualize(p.ec); err != nil {
		return &targetItem, sdk.ErrInternal("error actualizing double upgrade values: " + err.Error())
	} else {
		for idx, dbl := range dblKeyValues {
			dblKey, ok := targetItem.FindDoubleKey(dbl.Key)
			if !ok {
				return &targetItem, sdk.ErrInternal("double key does not exist which needs to be upgraded")
			}
			if len(toMod.Doubles[idx].Program) == 0 { // NO PROGRAM
				originValue := targetItem.Doubles[dblKey].Value.Float()
				upgradeAmount := dbl.Value.Float()
				targetItem.Doubles[dblKey].Value = types.ToFloatString(originValue + upgradeAmount)
			} else {
				targetItem.Doubles[dblKey].Value = dbl.Value
			}
		}
	}

	if lngKeyValues, err := toMod.Longs.Actualize(p.ec); err != nil {
		return &targetItem, sdk.ErrInternal("error actualizing long upgrade values: " + err.Error())
	} else {
		for idx, lng := range lngKeyValues {
			lngKey, ok := targetItem.FindLongKey(lng.Key)
			if !ok {
				return &targetItem, sdk.ErrInternal("long key does not exist which needs to be upgraded")
			}
			if len(toMod.Longs[idx].Program) == 0 { // NO PROGRAM
				targetItem.Longs[lngKey].Value += lng.Value
			} else {
				targetItem.Longs[lngKey].Value = lng.Value
			}
		}
	}

	if strKeyValues, err := toMod.Strings.Actualize(p.ec); err != nil {
		return &targetItem, sdk.ErrInternal("error actualizing string upgrade values: " + err.Error())
	} else {
		for _, str := range strKeyValues {
			strKey, ok := targetItem.FindStringKey(str.Key)
			if !ok {
				return &targetItem, sdk.ErrInternal("string key does not exist which needs to be upgraded")
			}
			targetItem.Strings[strKey].Value = str.Value
		}
	}

	// after upgrading is done, OwnerRecipe is not set
	targetItem.OwnerRecipeID = ""

	return &targetItem, nil
}

func (p *ExecProcess) GenerateCelEnvVarFromInputItems() error {
	// create environment variables from matched items
	varDefs := [](*exprpb.Decl){}
	variables := map[string]interface{}{}
	for idx, item := range p.matchedItems {
		iPrefix := fmt.Sprintf("input%d.", idx)
		for _, dbli := range item.Doubles {
			varDefs = append(varDefs, decls.NewIdent(iPrefix+dbli.Key, decls.Double, nil))
			variables[iPrefix+dbli.Key] = dbli.Value.Float() // input0.attack
			if idx == 0 {
				varDefs = append(varDefs, decls.NewIdent(dbli.Key, decls.Double, nil))
				variables[dbli.Key] = dbli.Value.Float() // attack
			}
		}
		for _, inti := range item.Longs {
			varDefs = append(varDefs, decls.NewIdent(iPrefix+inti.Key, decls.Int, nil))
			variables[iPrefix+inti.Key] = inti.Value // input0.level
			if idx == 0 {
				varDefs = append(varDefs, decls.NewIdent(inti.Key, decls.Int, nil))
				variables[inti.Key] = inti.Value // level
			}
		}
		for _, stri := range item.Strings {
			varDefs = append(varDefs, decls.NewIdent(iPrefix+stri.Key, decls.String, nil))
			variables[iPrefix+stri.Key] = stri.Value // input0.name
			if idx == 0 {
				varDefs = append(varDefs, decls.NewIdent(stri.Key, decls.String, nil))
				variables[stri.Key] = stri.Value // name
			}
		}
	}

	varDefs = append(varDefs,
		decls.NewFunction("rand_int",
			decls.NewOverload("rand_int",
				[]*exprpb.Type{decls.Int},
				decls.Int),
		),
		decls.NewFunction("min_int",
			decls.NewOverload("min_int",
				[]*exprpb.Type{decls.Int, decls.Int},
				decls.Int),
		),
		decls.NewFunction("max_int",
			decls.NewOverload("max_int",
				[]*exprpb.Type{decls.Int, decls.Int},
				decls.Int),
		))

	funcs := cel.Functions(
		&functions.Overload{
			// operator for 1 param
			Operator: "rand_int",
			Unary: func(arg ref.Val) ref.Val {
				return celTypes.Int(rand.Intn(int(arg.Value().(int64))))
			},
		}, &functions.Overload{
			// operator for 2 param
			Operator: "min_int",
			Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
				lftInt64 := lhs.Value().(int64)
				rgtInt64 := rhs.Value().(int64)
				if lftInt64 > rgtInt64 {
					return celTypes.Int(rgtInt64)
				}
				return celTypes.Int(lftInt64)
			},
		}, &functions.Overload{
			// operator for 2 param
			Operator: "max_int",
			Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
				lftInt64 := lhs.Value().(int64)
				rgtInt64 := rhs.Value().(int64)
				if lftInt64 < rgtInt64 {
					return celTypes.Int(rgtInt64)
				}
				return celTypes.Int(lftInt64)
			},
		})

	env, err := cel.NewEnv(
		cel.Declarations(
			varDefs...,
		),
	)

	p.ec = types.NewCelEnvCollection(env, variables, funcs)
	return err
}