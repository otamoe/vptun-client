package client

import (
	"context"

	pb "github.com/otamoe/vptun-pb"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

func GetStatus(ctx context.Context) (status *pb.Status, err error) {

	var statusHost *pb.Status_Host
	if statusHost, err = getStatusHost(ctx); err != nil {
		return
	}
	var statusLoad *pb.Status_Load
	if statusLoad, err = getStatusLoad(ctx); err != nil {
		return
	}
	var statusCPU *pb.Status_CPU
	if statusCPU, err = getStatusCPU(ctx); err != nil {
		return
	}
	var statusMem *pb.Status_Mem
	if statusMem, err = getStatusMem(ctx); err != nil {
		return
	}
	var statusNet *pb.Status_Net
	if statusNet, err = getStatusNet(ctx); err != nil {
		return
	}
	var statusDisk *pb.Status_Disk
	if statusDisk, err = getStatusDisk(ctx); err != nil {
		return
	}

	status = &pb.Status{
		Host: statusHost,
		Load: statusLoad,
		Cpu:  statusCPU,
		Mem:  statusMem,
		Net:  statusNet,
		Disk: statusDisk,
	}
	return
}

func getStatusHost(ctx context.Context) (statusHost *pb.Status_Host, err error) {
	var infoStat *host.InfoStat
	if infoStat, err = host.InfoWithContext(ctx); err != nil {
		return
	}
	var temperatureStat []host.TemperatureStat
	if temperatureStat, err = host.SensorsTemperaturesWithContext(ctx); err != nil {
		return
	}

	statusHostTemperatures := make([]*pb.Status_Host_Temperature, len(temperatureStat))
	for i := 0; i < len(temperatureStat); i++ {
		temperaturStat := temperatureStat[i]
		statusHostTemperatures[i] = &pb.Status_Host_Temperature{
			SensorKey:      temperaturStat.SensorKey,
			Temperature:    temperaturStat.Temperature,
			SensorHigh:     temperaturStat.High,
			SensorCritical: temperaturStat.Critical,
		}
	}
	statusHost = &pb.Status_Host{
		Info: &pb.Status_Host_Info{
			Hostname:             infoStat.Hostname,
			Uptime:               infoStat.Uptime,
			BootTime:             infoStat.BootTime,
			Procs:                infoStat.Procs,
			Os:                   infoStat.OS,
			Platform:             infoStat.Platform,
			PlatformFamily:       infoStat.PlatformFamily,
			PlatformVersion:      infoStat.PlatformVersion,
			KernelVersion:        infoStat.KernelVersion,
			KernelArch:           infoStat.KernelArch,
			VirtualizationSystem: infoStat.VirtualizationSystem,
			VirtualizationRole:   infoStat.VirtualizationRole,
			HostId:               infoStat.HostID,
		},
		Temperatures: statusHostTemperatures,
	}
	return
}

func getStatusLoad(ctx context.Context) (statusLoad *pb.Status_Load, err error) {
	var loadStat *load.AvgStat
	if loadStat, err = load.AvgWithContext(ctx); err != nil {
		return
	}

	statusLoad = &pb.Status_Load{
		Load1:  loadStat.Load1,
		Load5:  loadStat.Load5,
		Load15: loadStat.Load15,
	}
	return
}
func getStatusCPU(ctx context.Context) (statusCPU *pb.Status_CPU, err error) {
	var infoStats []cpu.InfoStat
	if infoStats, err = cpu.InfoWithContext(ctx); err != nil {
		// return
	}
	var timeStats []cpu.TimesStat
	if timeStats, err = cpu.TimesWithContext(ctx, true); err != nil {
		return
	}

	statusCPUInfos := make([]*pb.Status_CPU_Info, len(infoStats))
	for i := 0; i < len(infoStats); i++ {
		infoStat := infoStats[i]
		statusCPUInfos[i] = &pb.Status_CPU_Info{
			Cpu:        infoStat.CPU,
			VendorId:   infoStat.VendorID,
			Family:     infoStat.Family,
			Model:      infoStat.Model,
			Stepping:   infoStat.Stepping,
			PhysicalId: infoStat.PhysicalID,
			CoreId:     infoStat.CoreID,
			Cores:      infoStat.Cores,
			ModelName:  infoStat.ModelName,
			Mhz:        infoStat.Mhz,
			CacheSize:  infoStat.CacheSize,
			Flags:      infoStat.Flags,
			Microcode:  infoStat.Microcode,
		}
	}

	statusCPUStats := make([]*pb.Status_CPU_Stat, len(timeStats))
	for i := 0; i < len(timeStats); i++ {
		timeStat := timeStats[i]
		statusCPUStats[i] = &pb.Status_CPU_Stat{
			Cpu:       timeStat.CPU,
			User:      timeStat.User,
			System:    timeStat.System,
			Idle:      timeStat.Idle,
			Nice:      timeStat.Nice,
			Iowait:    timeStat.Iowait,
			Irq:       timeStat.Irq,
			Softirq:   timeStat.Softirq,
			Steal:     timeStat.Steal,
			Guest:     timeStat.Guest,
			GuestNice: timeStat.GuestNice,
		}
	}

	statusCPU = &pb.Status_CPU{
		Infos: statusCPUInfos,
		Stats: statusCPUStats,
	}
	return
}

func getStatusMem(ctx context.Context) (statusMem *pb.Status_Mem, err error) {
	var virtualMemoryStat *mem.VirtualMemoryStat
	if virtualMemoryStat, err = mem.VirtualMemoryWithContext(ctx); err != nil {
		return
	}
	var swapMemoryStat *mem.SwapMemoryStat
	if swapMemoryStat, err = mem.SwapMemoryWithContext(ctx); err != nil {
		return
	}
	statusMem = &pb.Status_Mem{
		Virtual: &pb.Status_Mem_Virtual{
			Total:       virtualMemoryStat.Total,
			Available:   virtualMemoryStat.Available,
			Used:        virtualMemoryStat.Used,
			UsedPercent: virtualMemoryStat.UsedPercent,
			Free:        virtualMemoryStat.Free,
		},
		Swap: &pb.Status_Mem_Swap{
			Total:       swapMemoryStat.Total,
			Used:        swapMemoryStat.Used,
			Free:        swapMemoryStat.Free,
			UsedPercent: swapMemoryStat.UsedPercent,
			Sin:         swapMemoryStat.Sin,
			Sout:        swapMemoryStat.Sout,
			PgIn:        swapMemoryStat.PgIn,
			PgOut:       swapMemoryStat.PgOut,
			PgFault:     swapMemoryStat.PgFault,
		},
	}
	return
}

func getStatusNet(ctx context.Context) (statusNet *pb.Status_Net, err error) {
	var ioCountersStats []net.IOCountersStat
	if ioCountersStats, err = net.IOCountersWithContext(ctx, false); err != nil {
		return
	}
	var interfaceStatList net.InterfaceStatList
	if interfaceStatList, err = net.InterfacesWithContext(ctx); err != nil {
		return
	}

	statusNetInfos := make([]*pb.Status_Net_Info, len(interfaceStatList))
	for i, interfaceStat := range interfaceStatList {
		var addrList []string
		for _, addr := range interfaceStat.Addrs {
			addrList = append(addrList, addr.Addr)
		}
		statusNetInfos[i] = &pb.Status_Net_Info{
			Index:        int32(interfaceStat.Index),
			Mtu:          int32(interfaceStat.MTU),
			Name:         interfaceStat.Name,
			HardwareAddr: interfaceStat.HardwareAddr,
			Flags:        interfaceStat.Flags,
			AddrList:     addrList,
		}
	}

	statusNetStats := make([]*pb.Status_Net_Stat, len(ioCountersStats))
	for i, ioCountersStat := range ioCountersStats {
		statusNetStats[i] = &pb.Status_Net_Stat{
			Name:        ioCountersStat.Name,
			BytesSent:   ioCountersStat.BytesSent,
			BytesRecv:   ioCountersStat.BytesRecv,
			PacketsSent: ioCountersStat.PacketsSent,
			PacketsRecv: ioCountersStat.PacketsRecv,
			Errin:       ioCountersStat.Errin,
			Errout:      ioCountersStat.Errout,
			Dropin:      ioCountersStat.Dropin,
			Dropout:     ioCountersStat.Dropout,
			Fifoin:      ioCountersStat.Fifoin,
			Fifoout:     ioCountersStat.Fifoout,
		}
	}

	statusNet = &pb.Status_Net{
		Infos: statusNetInfos,
		Stats: statusNetStats,
	}

	return
}

func getStatusDisk(ctx context.Context) (statusDisk *pb.Status_Disk, err error) {
	var partitions []disk.PartitionStat
	if partitions, err = disk.PartitionsWithContext(ctx, true); err != nil {
		return
	}
	statusDiskInfos := make([]*pb.Status_Disk_Info, len(partitions))
	statusDiskStats := make([]*pb.Status_Disk_Stat, len(partitions))
	partitionNames := make([]string, len(partitions))
	for i, partition := range partitions {
		partitionNames[i] = partition.Mountpoint
		statusDiskInfos[i] = &pb.Status_Disk_Info{
			Device:     partition.Device,
			Mountpoint: partition.Mountpoint,
			Fstype:     partition.Fstype,
		}
		var usageStat *disk.UsageStat
		if usageStat, err = disk.UsageWithContext(ctx, partition.Mountpoint); err != nil {
			return
		}
		statusDiskStats[i] = &pb.Status_Disk_Stat{
			Path:              usageStat.Path,
			Fstype:            usageStat.Fstype,
			Total:             usageStat.Total,
			Free:              usageStat.Free,
			Used:              usageStat.Used,
			UsedPercent:       usageStat.UsedPercent,
			InodesTotal:       usageStat.InodesTotal,
			InodesUsed:        usageStat.InodesUsed,
			InodesFree:        usageStat.InodesFree,
			InodesUsedPercent: usageStat.InodesUsedPercent,
		}
	}
	var ioCountersStats map[string]disk.IOCountersStat
	if ioCountersStats, err = disk.IOCountersWithContext(ctx, partitionNames...); err != nil {
		return
	}
	statusDiskIos := make([]*pb.Status_Disk_IO, len(ioCountersStats))
	i := 0
	for _, ioCountersStat := range ioCountersStats {
		statusDiskIos[i] = &pb.Status_Disk_IO{
			ReadCount:        ioCountersStat.ReadCount,
			MergedReadCount:  ioCountersStat.MergedReadCount,
			WriteCount:       ioCountersStat.WriteCount,
			MergedWriteCount: ioCountersStat.MergedWriteCount,
			ReadBytes:        ioCountersStat.ReadBytes,
			WriteBytes:       ioCountersStat.WriteBytes,
			ReadTime:         ioCountersStat.ReadTime,
			WriteTime:        ioCountersStat.WriteTime,
			IopsInProgress:   ioCountersStat.IopsInProgress,
			IoTime:           ioCountersStat.IoTime,
			WeightedIO:       ioCountersStat.WeightedIO,
			Name:             ioCountersStat.Name,
			SerialNumber:     ioCountersStat.SerialNumber,
			Label:            ioCountersStat.Label,
		}
		i++
	}

	statusDisk = &pb.Status_Disk{
		Infos: statusDiskInfos,
		Stats: statusDiskStats,
		Ios:   statusDiskIos,
	}
	return
}
