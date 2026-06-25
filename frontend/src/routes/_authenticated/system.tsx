import { createFileRoute } from '@tanstack/react-router';
import { SystemSettingsPage } from '@/features/system/SystemSettingsPage';

export const Route = createFileRoute('/_authenticated/system')({
  component: SystemSettingsPage,
});
