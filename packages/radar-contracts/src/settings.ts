import type { components } from './openapi';

type GeneratedDetectionSettings = components['schemas']['DetectionSettingsRead'];
type GeneratedWorkSchedule = components['schemas']['WorkScheduleRead'];

export interface DetectionSettings extends Omit<GeneratedDetectionSettings, 'keywords'> {
  keywords: string[];
}

export type DetectionSettingsUpdate = DetectionSettings;

export type WorkScheduleSlot = components['schemas']['WorkScheduleSlot'];

export interface WorkSchedule extends Omit<GeneratedWorkSchedule, 'slots'> {
  slots: WorkScheduleSlot[];
}

export type WorkScheduleUpdate = Omit<WorkSchedule, 'isDefault'>;
export type NotificationSettings = components['schemas']['NotificationSettingsRead'];
export type NotificationSettingsUpdate = components['schemas']['NotificationSettingsUpdate'];
export type SettingsCapabilities = components['schemas']['SettingsCapabilitiesRead'];

export interface SettingsBundle {
  detection: DetectionSettings;
  workSchedule: WorkSchedule;
  notifications: NotificationSettings;
  capabilities: SettingsCapabilities;
}
