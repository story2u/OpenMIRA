package com.codeiy.im

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Inbox
import androidx.compose.material.icons.filled.Work
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import androidx.navigation.navDeepLink
import com.codeiy.im.core.auth.SessionState
import com.codeiy.im.core.auth.SessionStore
import com.codeiy.im.core.auth.TokenStore
import com.codeiy.im.core.billing.RevenueCatBillingService
import com.codeiy.im.feature.dashboard.DashboardScreen
import com.codeiy.im.feature.login.LoginScreen
import com.codeiy.im.feature.jobs.JobDetailScreen
import com.codeiy.im.feature.jobs.JobDiscoveryScreen
import com.codeiy.im.feature.jobs.JobSearchProfilesScreen
import com.codeiy.im.feature.opportunity.OpportunityDetailScreen
import com.codeiy.im.feature.settings.DetectionSettingsScreen
import com.codeiy.im.feature.settings.NotificationSettingsScreen
import com.codeiy.im.feature.settings.PasswordSecurityScreen
import com.codeiy.im.feature.settings.SettingsRoute
import com.codeiy.im.feature.settings.SettingsScreen
import com.codeiy.im.feature.settings.SettingsViewModel
import com.codeiy.im.feature.settings.TelegramSettingsScreen
import com.codeiy.im.feature.settings.WorkScheduleScreen
import com.codeiy.im.feature.subscription.SubscriptionScreen
import com.codeiy.im.ui.theme.RadarTheme

class MainActivity : ComponentActivity() {
    private val session: SessionStore by viewModels {
        object : ViewModelProvider.Factory {
            @Suppress("UNCHECKED_CAST")
            override fun <T : ViewModel> create(modelClass: Class<T>): T =
                SessionStore(TokenStore(applicationContext), RevenueCatBillingService(applicationContext)) as T
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            RadarTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    RootView(session)
                }
            }
        }
    }
}

@Composable
private fun RootView(session: SessionStore) {
    val state by session.state.collectAsState()
    when (val current = state) {
        is SessionState.Restoring -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator()
        }
        is SessionState.RestoreFailed -> Column(
            modifier = Modifier.fillMaxSize().padding(24.dp),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text("会话恢复失败", style = MaterialTheme.typography.titleMedium)
            Text(current.message, style = MaterialTheme.typography.bodyMedium)
            Button(onClick = { session.restore() }) { Text("重试") }
            TextButton(onClick = { session.logout() }) { Text("退出登录") }
        }
        is SessionState.LoggedOut -> LoginScreen(session)
        is SessionState.Active -> AppNavHost(session)
    }
}

@Composable
private fun AppNavHost(session: SessionStore) {
    val navController = rememberNavController()
    NavHost(navController, startDestination = "main") {
        // 一级 Tab 容器（商机/工作机会/设置），各自内部导航到详情/设置子页。
        composable("main") {
            MainTabs(
                session = session,
                onOpenOpportunity = { id -> navController.navigate("opportunity/$id") },
                onOpenJob = { id, profileId ->
                    navController.navigate("job/$id${profileId?.let { "?profileId=$it" }.orEmpty()}")
                },
                onOpenSettingsRoute = { route -> navController.navigate(route) },
            )
        }
        composable(
            route = "opportunity/{opportunityId}",
            arguments = listOf(navArgument("opportunityId") { type = NavType.StringType }),
            // 推送/外链深链入口（对齐移动端计划「点击推送深链进详情」）。
            // https App Link 需在域名下发布 assetlinks.json 后才会直开 App，见 README。
            deepLinks = listOf(
                navDeepLink { uriPattern = "opportunity-radar://opportunity/{opportunityId}" },
                navDeepLink { uriPattern = "https://im.story2u.xyz/app/opportunity/{opportunityId}" },
            ),
        ) { entry ->
            val opportunityId = entry.arguments?.getString("opportunityId").orEmpty()
            OpportunityDetailScreen(
                session = session,
                opportunityId = opportunityId,
                onBack = { navController.popBackStack() },
            )
        }
        composable(
            route = "job/{jobId}?profileId={profileId}",
            arguments = listOf(
                navArgument("jobId") { type = NavType.StringType },
                navArgument("profileId") { type = NavType.StringType; nullable = true; defaultValue = null },
            ),
            deepLinks = listOf(
                navDeepLink { uriPattern = "opportunity-radar://job/{jobId}" },
                navDeepLink { uriPattern = "https://im.story2u.xyz/app/job/{jobId}" },
            ),
        ) { entry ->
            JobDetailScreen(
                session = session,
                opportunityId = entry.arguments?.getString("jobId").orEmpty(),
                profileId = entry.arguments?.getString("profileId"),
                onBack = { navController.popBackStack() },
            )
        }
        composable("settings/job-search") {
            JobSearchProfilesScreen(session = session, onBack = { navController.popBackStack() })
        }
        composable("settings/subscription") {
            SubscriptionScreen(session = session, onBack = { navController.popBackStack() })
        }
        composable("settings/security") {
            PasswordSecurityScreen(session = session, onBack = { navController.popBackStack() })
        }
        composable("settings/telegram") {
            TelegramSettingsScreen(session = session, onBack = { navController.popBackStack() })
        }
        composable("settings/detection") {
            DetectionSettingsRoute(session, onBack = { navController.popBackStack() })
        }
        composable("settings/work-schedule") {
            WorkScheduleRoute(session, onBack = { navController.popBackStack() })
        }
        composable("settings/notifications") {
            NotificationSettingsRoute(session, onBack = { navController.popBackStack() })
        }
    }
}

/** 三个一级 Tab：商机看板 / 工作机会 / 设置中心。 */
@Composable
private fun MainTabs(
    session: SessionStore,
    onOpenOpportunity: (String) -> Unit,
    onOpenJob: (String, String?) -> Unit,
    onOpenSettingsRoute: (String) -> Unit,
) {
    var selectedTab by rememberSaveable { mutableIntStateOf(0) }
    Scaffold(
        bottomBar = {
            NavigationBar {
                NavigationBarItem(
                    selected = selectedTab == 0,
                    onClick = { selectedTab = 0 },
                    icon = { Icon(Icons.Filled.Inbox, contentDescription = null) },
                    label = { Text("商机") },
                )
                NavigationBarItem(
                    selected = selectedTab == 1,
                    onClick = { selectedTab = 1 },
                    icon = { Icon(Icons.Filled.Work, contentDescription = null) },
                    label = { Text("工作机会") },
                )
                NavigationBarItem(
                    selected = selectedTab == 2,
                    onClick = { selectedTab = 2 },
                    icon = { Icon(Icons.Filled.Settings, contentDescription = null) },
                    label = { Text("设置") },
                )
            }
        },
    ) { padding ->
        Box(Modifier.padding(padding)) {
            when (selectedTab) {
                0 -> DashboardScreen(session, onOpenOpportunity = onOpenOpportunity)
                1 -> JobDiscoveryScreen(
                    session = session,
                    onOpenJob = onOpenJob,
                    onOpenProfiles = { onOpenSettingsRoute("settings/job-search") },
                )
                else -> SettingsScreen(session) { route ->
                    when (route) {
                        SettingsRoute.Security -> onOpenSettingsRoute("settings/security")
                        SettingsRoute.Subscription -> onOpenSettingsRoute("settings/subscription")
                        SettingsRoute.Telegram -> onOpenSettingsRoute("settings/telegram")
                        SettingsRoute.Detection -> onOpenSettingsRoute("settings/detection")
                        SettingsRoute.WorkSchedule -> onOpenSettingsRoute("settings/work-schedule")
                        SettingsRoute.Notifications -> onOpenSettingsRoute("settings/notifications")
                        SettingsRoute.JobSearch -> onOpenSettingsRoute("settings/job-search")
                    }
                }
            }
        }
    }
}

// 设置子页 Route 包装：读取已加载的 bundle 交给对应屏。
@Composable
private fun DetectionSettingsRoute(session: SessionStore, onBack: () -> Unit) {
    val model: SettingsViewModel = viewModel { SettingsViewModel(session.api.service) }
    val state by model.state.collectAsStateWithLifecycle()
    LaunchedEffect(Unit) { model.load() }
    state.bundle?.let { DetectionSettingsScreen(model, it.detection, onBack) }
        ?: LoadingOrError(state.loadError, onBack)
}

@Composable
private fun WorkScheduleRoute(session: SessionStore, onBack: () -> Unit) {
    val model: SettingsViewModel = viewModel { SettingsViewModel(session.api.service) }
    val state by model.state.collectAsStateWithLifecycle()
    LaunchedEffect(Unit) { model.load() }
    state.bundle?.let { WorkScheduleScreen(model, it.workSchedule, onBack) }
        ?: LoadingOrError(state.loadError, onBack)
}

@Composable
private fun NotificationSettingsRoute(session: SessionStore, onBack: () -> Unit) {
    val model: SettingsViewModel = viewModel { SettingsViewModel(session.api.service) }
    val state by model.state.collectAsStateWithLifecycle()
    LaunchedEffect(Unit) { model.load() }
    state.bundle?.let { NotificationSettingsScreen(model, it.notifications, it.capabilities.pushAvailable, onBack) }
        ?: LoadingOrError(state.loadError, onBack)
}

@Composable
private fun LoadingOrError(error: String?, onBack: () -> Unit) {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        if (error != null) {
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                Text(error)
                TextButton(onClick = onBack) { Text("返回") }
            }
        } else {
            CircularProgressIndicator()
        }
    }
}
