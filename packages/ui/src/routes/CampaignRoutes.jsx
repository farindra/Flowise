import { lazy } from 'react'
import Loadable from '@/ui-component/loading/Loadable'

const CampaignLanding = Loadable(lazy(() => import('@/views/campaign-landing')))

// Route publik — tidak butuh auth, tidak pakai MainLayout
const CampaignRoutes = {
    path: '/lp',
    children: [
        {
            path: ':slug',
            element: <CampaignLanding />
        }
    ]
}

export default CampaignRoutes
