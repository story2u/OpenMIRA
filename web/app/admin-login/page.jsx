import { AppShell } from "../../components/AppShell.jsx";
import { LoginPageClient } from "../../components/LoginPageClient.jsx";

export default function AdminLoginPage() {
  return (
    <AppShell active="admin">
      <LoginPageClient mode="admin" />
    </AppShell>
  );
}
