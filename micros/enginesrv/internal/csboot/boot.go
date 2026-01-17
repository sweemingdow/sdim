package csboot

import (
	"context"
	"github.com/sweemingdow/gmicro_pkg/pkg/boot"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cnsq"
	"github.com/sweemingdow/sdim/external/econfig"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst"
	"github.com/sweemingdow/sdim/external/erpc/rpctopic"
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/codec/fcodec"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/config/esncfg"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core/connmgr"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core/frhandler"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core/mqhandler/convadd"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core/mqhandler/convupdate"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core/mqhandler/msgforward"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/drive"
)

func StartConnDriveEngine(ac *boot.AppContext, sc esncfg.StaticConfig) {
	de := sc.DriveEngine
	cmCfg := sc.ConnManager
	nsqCfg := sc.NsqCfg
	cm := connmgr.NewConnManager(
		cmCfg.EstimatedCapacity,
		cmCfg.ConcurrencyStrip,
		cmCfg.UserMaxSlaveConns,
		cmCfg.MasterClientTypes,
		cmCfg.AuthTimeout,
		cmCfg.PingInterval,
		cmCfg.PingLostTimes,
	)

	frCodec := fcodec.NewFrameCodec()
	pdCfg, csCfg := econfig.NsqCfgConvert(nsqCfg)

	nsqPd, err := cnsq.NewNsqProducer(pdCfg)
	if err != nil {
		ac.GetEc() <- err
		return
	}

	nsqFactory := cnsq.NewStaticNsqMsgConsumeFactory()
	nsqFactory.Register(nsqconst.MsgForwardTopic, msgforward.NewMsgForwardHandler(cm, frCodec))
	nsqFactory.Register(nsqconst.ConvUpdateTopic, convupdate.NewConvUpdateHandler(cm, frCodec))
	nsqFactory.Register(nsqconst.ConvAddTopic, convadd.NewConvAddHandler(cm, frCodec))

	nsdCs, err := cnsq.NewNsqConsumer(csCfg, nsqFactory)
	if err != nil {
		ac.GetEc() <- err
		return
	}

	cde := drive.NewConnDriveEngine(
		de.TcpPort,
		de.Multicore,
		de.BatchRead,
		frCodec,
		frhandler.NewAsyncFrameHandler(
			frCodec,
			cm,
			nsqPd,
			rpctopic.NewTopicRpcProvider(ac.GetArpcClientFactory()),
			rpcuser.NewUserInfoRpcProvider(ac.GetArpcClientFactory()),
		),
		cm,
	)

	ac.CollectLifecycle("conn_drive_engine", cde)

	ac.CollectLifecycle(cnsq.ProducerLifetimeTag, nsqPd)

	ac.CollectLifecycle(cnsq.ConsumerLifetimeTag, nsdCs)

	go func() {
		if e := cde.Run(); e != nil {
			ac.GetEc() <- e
		}
	}()
}

func ShutdownConnDriveEngine(ac *boot.AppContext, ctx context.Context) error {
	//lg := mylog.AppLoggerWithStop()
	//cdeVal := ac.GetStored("conn_drive_engine")
	//
	//if cde, ok := cdeVal.(drive.ConnDriveEngine); ok {
	//	if err := cde.GracefulStop(ctx); err != nil {
	//
	//		lg.Error().Stack().Err(err).Msg("stop connection drive engine failed")
	//		return err
	//	}
	//
	//	lg.Info().Msg("stop connection drive engine successfully")
	//}
	//
	//nsqPdVal := ac.GetStored("nsq_producer")
	//if nsqPd, ok := nsqPdVal.(*cnsq.NsqProducer); ok {
	//	if err := nsqPd.GracefulStop(ctx); err != nil {
	//
	//		lg.Error().Stack().Err(err).Msg("stop nsq producer failed")
	//		return err
	//	}
	//
	//	lg.Info().Msg("stop nsq producer successfully")
	//}
	//
	//nsqCsVal := ac.GetStored("nsq_consumer")
	//if nsqCs, ok := nsqCsVal.(*cnsq.NsqConsumer); ok {
	//	if err := nsqCs.GracefulStop(ctx); err != nil {
	//
	//		lg.Error().Stack().Err(err).Msg("stop nsq consumer failed")
	//		return err
	//	}
	//
	//	lg.Info().Msg("stop nsq consumer successfully")
	//}

	return nil
}
