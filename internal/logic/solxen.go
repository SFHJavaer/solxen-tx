package logic

import (
	"context"
	"fmt"
	"solxen-tx/internal/config"
	"time"

	"solxen-tx/internal/logic/generated/sol_xen"

	"github.com/davecgh/go-spew/spew"
	"github.com/ethereum/go-ethereum/common"
	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/errorx"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/mr"
)

func NewXnRecordAddress(config config.Config) solana.PublicKey {
	programPubKey, _ := solana.PublicKeyFromBase58(config.Sol.ProgramID)

	seed := [][]byte{
		[]byte("xn-global-counter"),
	}
	xnRecordAddress, _, err := solana.FindProgramAddress(seed, programPubKey)
	if err != nil {
		logx.Errorf("err :%v", err)
	}

	return xnRecordAddress
}

func (l *Producer) GetglobalXnRecord() ([]byte, error) {
	var inVar sol_xen.GlobalXnRecord
	resp, err := l.svcCtx.SolCli.GetAccountInfo(l.ctx,
		solana.MustPublicKeyFromBase58(l.svcCtx.Config.Sol.ProgramID),
	)
	if err != nil {
		return nil, err
	}
	bin.NewBinDecoder(resp.Value.Data.GetBinary()).Decode(&inVar)
	spew.Dump(inVar)
	return nil, err
}

func (l *Producer) Mint() error {
	t := time.Now()
	var (
		programId = solana.MustPublicKeyFromBase58(l.svcCtx.Config.Sol.ProgramID)
		seed      = [][]byte{[]byte("xn-global-counter")}
	)
	globalXnRecordAddress, err := l.FindProgramAddressSync(seed, programId)
	if err != nil {
		return errorx.Wrap(err, "globalXnRecordAddress")
	}

	seed = [][]byte{[]byte("mint")}
	mint, err := l.FindProgramAddressSync(seed, programId)
	if err != nil {
		return errorx.Wrap(err, "mint")
	}
	var mintAccount token.Mint
	err = l.svcCtx.SolCli.GetAccountDataInto(context.TODO(), mint, &mintAccount)
	if err != nil {
		return errorx.Wrap(err, "mintAccount")
	}

	associateTokenProgram := solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")
	user := solana.MustPrivateKeyFromBase58(l.svcCtx.Config.Sol.Key)
	userTokenAccount, _, err := solana.FindAssociatedTokenAddress(
		user.PublicKey(),
		mint,
	)

	var (
		fromAddr string
	)
	if common.IsHexAddress(l.svcCtx.Config.Sol.ToAddr) {
		fromAddr = l.svcCtx.Config.Sol.ToAddr[2:]
	}
	seed = [][]byte{[]byte("sol-xen"), common.FromHex(fromAddr), user.PublicKey().Bytes()}
	userXnRecordAccount, _, err := solana.FindProgramAddress(seed, programId)
	if err != nil {
		return errorx.Wrap(err, "userXnRecordAccount")
	}

	var globalXnRecordNew sol_xen.GlobalXnRecord
	seed = [][]byte{[]byte("sol-xen-addr"), common.FromHex(fromAddr)}
	info, err := l.svcCtx.SolCli.GetAccountInfoWithOpts(l.ctx, globalXnRecordAddress, &rpc.GetAccountInfoOpts{
		Commitment: rpc.CommitmentConfirmed})
	err = bin.NewBinDecoder(info.GetBinary()).Decode(&globalXnRecordNew)
	if err != nil {
		return errorx.Wrap(err, "globalXnRecordNew")
	}

	ethAccount := common.HexToAddress(l.svcCtx.Config.Sol.ToAddr)
	var uint8Array [20]uint8
	copy(uint8Array[:], ethAccount[:])
	eth := sol_xen.EthAccount{}
	eth.Address = uint8Array

	mintToken := sol_xen.NewMintTokensInstructionBuilder().
		SetEthAccount(eth).
		SetUserTokenAccountAccount(userTokenAccount).
		SetGlobalXnRecordAccount(globalXnRecordAddress).
		SetUserXnRecordAccount(userXnRecordAccount).
		SetUserAccount(user.PublicKey()).
		SetMintAccountAccount(mint).
		SetTokenProgramAccount(solana.TokenProgramID).
		SetSystemProgramAccount(solana.SystemProgramID).
		SetAssociatedTokenProgramAccount(associateTokenProgram).
		SetRentAccount(solana.SysVarRentPubkey).Build()

	sol_xen.SetProgramID(solana.MustPublicKeyFromBase58(l.svcCtx.Config.Sol.ProgramID))
	data, _ := mintToken.Data()
	instruction := solana.NewInstruction(mintToken.ProgramID(), mintToken.Accounts(), data)
	signers := []solana.PrivateKey{user}

	limit := computebudget.NewSetComputeUnitLimitInstruction(1400000).Build()
	feesInit := computebudget.NewSetComputeUnitPriceInstructionBuilder().SetMicroLamports(l.svcCtx.Config.Sol.Fee).Build()

	recent, err := l.svcCtx.SolCli.GetLatestBlockhash(context.Background(), rpc.CommitmentFinalized)
	rent := recent.Value.Blockhash

	tx, err := solana.NewTransactionBuilder().
		AddInstruction(feesInit).
		AddInstruction(limit).
		AddInstruction(instruction).
		SetRecentBlockHash(rent).
		SetFeePayer(user.PublicKey()).
		Build()
	if err != nil {
		return errorx.Wrap(err, "tx")
	}

	// tx.EncodeTree(text.NewTreeEncoder(os.Stdout, "Transfer SOL"))

	_, err = tx.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			for _, signer := range signers {
				if signer.PublicKey().Equals(key) {
					return &signer
				}
			}
			return nil
		},
	)
	if err != nil {
		return errorx.Wrap(err, "Sign")
	}

	sig, err := l.svcCtx.SolCli.SendTransaction(context.TODO(), tx)
	if err != nil {
		return errorx.Wrap(err, "sig")
	}
	userTokenBalance, err := l.svcCtx.SolCli.GetTokenAccountBalance(l.ctx, userTokenAccount, rpc.CommitmentConfirmed)
	if err != nil {
		return errorx.Wrap(err, "userTokenBalance")
	}
	logx.Infof("Tx: %v hashes: %v superhashes: %v balance: %v t: %v", sig.String(), globalXnRecordNew.Hashes,
		globalXnRecordNew.Superhashes, userTokenBalance.Value.UiAmountString, time.Since(t))
	return nil
}

func (l *Producer) FindProgramAddressSync(seeds [][]byte, programId solana.PublicKey) (solana.PublicKey, error) {
	resp, _, err := solana.FindProgramAddress(seeds, programId)
	return resp, err
}

func (l *Producer) CheckAddressBalance() error {

	var (
		fns []func() error
	)
	for _, addr := range l.svcCtx.AddrList {
		fns = append(fns, func() error {
			balance, err := l.svcCtx.SolCli.GetBalance(l.ctx, addr.PublicKey(), rpc.CommitmentFinalized)
			if err != nil {
				return err
			}
			if (balance.Value) < 1_000_000 {
				return errorx.Wrap(err, fmt.Sprintf("%v Balance less than 0.01, please recharge.余额小于0.01请充值", addr.PublicKey()))
			}
			return nil
		})

	}
	err := mr.Finish(
		fns...,
	)
	if err != nil {
		logx.Errorf("err %v", err)
	}
	return nil
}

func (l *Producer) QueryNetWorkGas() error {
	return nil
}

func (l *Producer) ListTxpoolPendding() error {
	return nil
}
