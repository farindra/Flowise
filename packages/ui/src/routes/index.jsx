import { useRoutes } from 'react-router-dom'

// routes
import MainRoutes from './MainRoutes'
import CanvasRoutes from './CanvasRoutes'
import ChatbotRoutes from './ChatbotRoutes'
import config from '@/config'
import AuthRoutes from '@/routes/AuthRoutes'
import ExecutionRoutes from './ExecutionRoutes'
import CampaignRoutes from './CampaignRoutes'

// ==============================|| ROUTING RENDER ||============================== //

export default function ThemeRoutes() {
    return useRoutes([CampaignRoutes, MainRoutes, AuthRoutes, CanvasRoutes, ChatbotRoutes, ExecutionRoutes], config.basename)
}
