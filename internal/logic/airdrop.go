package logic

import (
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
)

func (l *Producer) Airdrop() {
	for _, account := range l.svcCtx.AddrList {
		out, err := l.svcCtx.SolCli.RequestAirdrop(
			l.ctx,
			account.PublicKey(),
			solana.LAMPORTS_PER_SOL*100,
			rpc.CommitmentFinalized,
		)
		if err != nil {
			logx.Errorf("err :%v", err)
		}
		logx.Infof("signature: %v amount:%v account:%v    ", out.String(), account.PublicKey(), 100)
		time.Sleep(1)
	}

}
