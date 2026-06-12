package gpmc

type DeviceProfile struct {
	Model             string
	Make              string
	BuildID           string
	AndroidAPILevel   int
	ClientVersionCode int
}

func DefaultPixelXL() DeviceProfile {
	return DeviceProfile{
		Model:             "Pixel XL",
		Make:              "Google",
		BuildID:           "PQ2A.190205.001",
		AndroidAPILevel:   28,
		ClientVersionCode: 49029607,
	}
}

func DefaultPixel5() DeviceProfile {
	return DeviceProfile{
		Model:             "Pixel 5",
		Make:              "Google",
		BuildID:           "RQ3A.210805.001.A1",
		AndroidAPILevel:   30,
		ClientVersionCode: 49029607,
	}
}

func effectiveCommitModel(q Quality, _ DeviceProfile) string {
	switch q {
	case QualitySaver:
		return "Pixel 2"
	case QualityUseQuota:
		return "Pixel 8"
	default:
		return "Pixel XL"
	}
}

func commitQualityCode(q Quality) int32 {
	if q == QualitySaver {
		return 1
	}
	return 3
}
