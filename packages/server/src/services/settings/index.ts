// TODO: add settings

import { Platform } from '../../Interface'
import { getRunningExpressApp } from '../../utils/getRunningExpressApp'

const getSettings = async () => {
    try {
        const appServer = getRunningExpressApp()
        const platformType = appServer.identityManager.getPlatformType()

        const appName = process.env.APP_NAME || 'Farindra Agentic'

        switch (platformType) {
            case Platform.ENTERPRISE: {
                if (!appServer.identityManager.isLicenseValid()) {
                    return { APP_NAME: appName }
                } else {
                    return { PLATFORM_TYPE: Platform.ENTERPRISE, APP_NAME: appName }
                }
            }
            case Platform.CLOUD: {
                return { PLATFORM_TYPE: Platform.CLOUD, APP_NAME: appName }
            }
            default: {
                return { PLATFORM_TYPE: Platform.OPEN_SOURCE, APP_NAME: appName }
            }
        }
    } catch (error) {
        return {}
    }
}

export default {
    getSettings
}
