package gpmc

import "fmt"

const (
	endpointAuth         = "https://android.googleapis.com/auth"
	endpointUploadMedia  = "https://photos.googleapis.com/data/upload/uploadmedia/interactive"
	endpointFindByHash      = "https://photosdata-pa.googleapis.com/6439526531001121323/5084965799730810217"
	endpointCommitUpload    = "https://photosdata-pa.googleapis.com/6439526531001121323/16538846908252377752"
	endpointPrepareDownload = "https://photosdata-pa.googleapis.com/$rpc/social.frontend.photos.preparedownloaddata.v1.PhotosPrepareDownloadDataService/PhotosPrepareDownload"
	endpointLibState        = "https://photosdata-pa.googleapis.com/6439526531001121323/18047484249733410717"
	endpointDriveAbout      = "https://www.googleapis.com/drive/v3/about?fields=storageQuota"
	endpointStreamManifest  = "https://lh3.googleusercontent.com/p/%s%%3Dmm%%2C%s-vm"
	endpointThumbnail       = "https://ap2.googleusercontent.com/gpa/%s=k-sg-w%d-h%d-c-rj-no"

	extHeader173412678 = "CgcIAhClARgC"
	extHeader174067345 = "CgIIAg=="

	contentTypeProto = "application/x-protobuf"
	contentTypeForm  = "application/x-www-form-urlencoded"
)

func androidNameForAPI(api int) string {
	switch api {
	case 28:
		return "9"
	case 29:
		return "10"
	case 30:
		return "11"
	case 31:
		return "12"
	case 32:
		return "12"
	case 33:
		return "13"
	case 34:
		return "14"
	default:
		return fmt.Sprintf("%d", api)
	}
}

func apiUA(p DeviceProfile, language string) string {
	return fmt.Sprintf(
		"com.google.android.apps.photos/%d (Linux; U; Android %s; %s; %s; Build/%s; Cronet/127.0.6510.5) (gzip)",
		p.ClientVersionCode,
		androidNameForAPI(p.AndroidAPILevel),
		language,
		p.Model,
		p.BuildID,
	)
}

func authUA(p DeviceProfile) string {
	return fmt.Sprintf("GoogleAuth/1.4 (%s %s); gzip", p.Model, p.BuildID)
}
