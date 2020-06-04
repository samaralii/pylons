package pylons

import (
	"encoding/json"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"

	"github.com/Pylons-tech/pylons/x/pylons/client/cli/query"
	"github.com/Pylons-tech/pylons/x/pylons/client/cli/tx"
	"github.com/Pylons-tech/pylons/x/pylons/client/rest"
	"github.com/Pylons-tech/pylons/x/pylons/keep"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/bank"

	"github.com/cosmos/cosmos-sdk/client/context"
	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

// type check to ensure the interface is properly implemented
var (
	_ module.AppModule      = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
)

// app module Basics object
type AppModuleBasic struct{}

func (AppModuleBasic) Name() string {
	return "pylons"
}

func (AppModuleBasic) RegisterCodec(cdc *codec.Codec) {
	RegisterCodec(cdc)
}

func (AppModuleBasic) DefaultGenesis() json.RawMessage {
	return ModuleCdc.MustMarshalJSON(DefaultGenesisState())
}

// Validation check of the Genesis
func (AppModuleBasic) ValidateGenesis(bz json.RawMessage) error {
	var data GenesisState
	err := ModuleCdc.UnmarshalJSON(bz, &data)
	if err != nil {
		return err
	}
	// Once json successfully marshalled, passes along to genesis.go
	return ValidateGenesis(data)
}

// RegisterRESTRoutes rest routes
func (AppModuleBasic) RegisterRESTRoutes(ctx context.CLIContext, rtr *mux.Router) {
	rest.RegisterRoutes(ctx, rtr, ModuleCdc, StoreKey)
}

// Get the root query command of this module
func (AppModuleBasic) GetQueryCmd(cdc *codec.Codec) *cobra.Command {
	pylonsQueryCmd := &cobra.Command{
		Use:   "pylons",
		Short: "Querying commands for the pylons module",
	}
	pylonsQueryCmd.AddCommand(
		query.GetPylonsBalance(StoreKey, cdc),
		query.GetCookbook(StoreKey, cdc),
		query.GetExecution(StoreKey, cdc),
		query.GetItem(StoreKey, cdc),
		query.GetRecipe(StoreKey, cdc),
		query.ListCookbook(StoreKey, cdc),
		query.ListRecipes(StoreKey, cdc),
		query.ItemsBySender(StoreKey, cdc),
		query.ListExecutions(StoreKey, cdc),
		query.ListTrade(StoreKey, cdc))
	
	pylonsQueryCmd.PersistentFlags().String("node", "tcp://localhost:26657", "<host>:<port> to Tendermint RPC interface for this chain")

	return pylonsQueryCmd
}

// Get the root tx command of this module
func (AppModuleBasic) GetTxCmd(cdc *codec.Codec) *cobra.Command {
	pylonsTxCmd := &cobra.Command{
		Use:   "pylons",
		Short: "Pylons transactions subcommands",
	}

	pylonsTxCmd.AddCommand(
		tx.GetPylons(cdc),
		tx.SendPylons(cdc),
		tx.CreateCookbook(cdc),
		tx.UpdateCookbook(cdc),
		tx.FiatItem(cdc))
	
	pylonsTxCmd.PersistentFlags().String("node", "tcp://localhost:26657", "<host>:<port> to Tendermint RPC interface for this chain")
	pylonsTxCmd.PersistentFlags().String("keyring-backend", "os", "Select keyring's backend (os|file|test)")
	pylonsTxCmd.PersistentFlags().String("from", "", "Name or address of private key with which to sign")
	pylonsTxCmd.PersistentFlags().String("broadcast-mode", "sync", "Transaction broadcasting mode (sync|async|block)")

	return pylonsTxCmd
}

type AppModule struct {
	AppModuleBasic
	keeper     keep.Keeper
	bankKeeper bank.Keeper
}

// NewAppModule creates a new AppModule Object
func NewAppModule(k keep.Keeper, bankKeeper bank.Keeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keeper:         k,
		bankKeeper:     bankKeeper,
	}
}

func (AppModule) Name() string {
	return ModuleName
}

func (am AppModule) RegisterInvariants(ir sdk.InvariantRegistry) {}

func (am AppModule) Route() string {
	return RouterKey
}

func (am AppModule) NewHandler() sdk.Handler {
	return NewHandler(am.keeper)
}
func (am AppModule) QuerierRoute() string {
	return QuerierRoute
}

func (am AppModule) NewQuerierHandler() sdk.Querier {
	return NewQuerier(am.keeper)
}

func (am AppModule) BeginBlock(_ sdk.Context, _ abci.RequestBeginBlock) {}

func (am AppModule) EndBlock(sdk.Context, abci.RequestEndBlock) []abci.ValidatorUpdate {
	return []abci.ValidatorUpdate{}
}

func (am AppModule) InitGenesis(ctx sdk.Context, data json.RawMessage) []abci.ValidatorUpdate {
	var genesisState GenesisState
	ModuleCdc.MustUnmarshalJSON(data, &genesisState)
	InitGenesis(ctx, am.keeper, genesisState)
	return []abci.ValidatorUpdate{}
}

func (am AppModule) ExportGenesis(ctx sdk.Context) json.RawMessage {
	gs := ExportGenesis(ctx, am.keeper)
	return ModuleCdc.MustMarshalJSON(gs)
}