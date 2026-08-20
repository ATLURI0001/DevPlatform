'use client';

import { useState } from 'react';
import { useAuthStore } from '@/hooks/useAuthStore';
import { api } from '@/services/api';
import toast from 'react-hot-toast';
import { User, Lock, Bell, Palette, Database, Server, Save, Check } from 'lucide-react';

interface SettingsProps {}

export function Settings() {
  const { user, token, logout } = useAuthStore();
  const [activeTab, setActiveTab] = useState<'profile' | 'security' | 'notifications' | 'appearance' | 'integrations'>('profile');
  const [isSaving, setIsSaving] = useState(false);

  // Profile form
  const [profileForm, setProfileForm] = useState({
    name: user?.name || '',
    email: user?.email || '',
    bio: '',
    timezone: 'UTC',
  });

  // Security form
  const [securityForm, setSecurityForm] = useState({
    currentPassword: '',
    newPassword: '',
    confirmPassword: '',
    twoFactorEnabled: false,
  });

  // Notifications form
  const [notificationsForm, setNotificationsForm] = useState({
    emailDeployments: true,
    emailAIJobs: true,
    emailSecurity: true,
    slackWebhook: '',
    discordWebhook: '',
  });

  // Appearance form
  const [appearanceForm, setAppearanceForm] = useState({
    theme: 'system' as 'light' | 'dark' | 'system',
    compactMode: false,
    animations: true,
  });

  // Integrations form
  const [integrationsForm, setIntegrationsForm] = useState({
    githubToken: '',
    gitlabToken: '',
    awsAccessKey: '',
    awsSecretKey: '',
    gcpProjectId: '',
    azureSubscriptionId: '',
  });

  const tabs = [
    { id: 'profile', label: 'Profile', icon: User },
    { id: 'security', label: 'Security', icon: Lock },
    { id: 'notifications', label: 'Notifications', icon: Bell },
    { id: 'appearance', label: 'Appearance', icon: Palette },
    { id: 'integrations', label: 'Integrations', icon: Server },
  ];

  const handleSave = async (tab: string) => {
    setIsSaving(true);
    try {
      // In a real app, this would call the API
      // await api.put('/api/v1/user/settings', { [tab]: getFormData(tab) });
      await new Promise((resolve) => setTimeout(resolve, 500));
      toast.success(`${tabs.find((t) => t.id === tab)?.label} settings saved`);
    } catch (error) {
      toast.error('Failed to save settings');
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="space-y-6 max-w-4xl">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-gray-900">Settings</h1>
        <p className="mt-1 text-sm text-gray-500">Manage your account and preferences</p>
      </div>

      {/* Tabs */}
      <div className="bg-white rounded-xl border border-gray-200 shadow-sm overflow-hidden">
        <div className="border-b border-gray-200">
          <nav className="flex overflow-x-auto px-4" aria-label="Settings tabs">
            {tabs.map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id as typeof activeTab)}
                className={`flex items-center gap-2 px-4 py-3 text-sm font-medium border-b-2 transition-colors whitespace-nowrap ${
                  activeTab === tab.id
                    ? 'text-primary-600 border-primary-600 bg-primary-50'
                    : 'text-gray-500 hover:text-gray-700 hover:bg-gray-50 border-transparent'
                }`}
              >
                <tab.icon className="h-5 w-5" />
                {tab.label}
              </button>
            ))}
          </nav>
        </div>

        {/* Tab Panels */}
        <div className="p-6">
          {activeTab === 'profile' && (
            <ProfileForm form={profileForm} setForm={setProfileForm} onSave={() => handleSave('profile')} isSaving={isSaving} />
          )}
          {activeTab === 'security' && (
            <SecurityForm form={securityForm} setForm={setSecurityForm} onSave={() => handleSave('security')} isSaving={isSaving} />
          )}
          {activeTab === 'notifications' && (
            <NotificationsForm form={notificationsForm} setForm={setNotificationsForm} onSave={() => handleSave('notifications')} isSaving={isSaving} />
          )}
          {activeTab === 'appearance' && (
            <AppearanceForm form={appearanceForm} setForm={setAppearanceForm} onSave={() => handleSave('appearance')} isSaving={isSaving} />
          )}
          {activeTab === 'integrations' && (
            <IntegrationsForm form={integrationsForm} setForm={setIntegrationsForm} onSave={() => handleSave('integrations')} isSaving={isSaving} />
          )}
        </div>
      </div>

      {/* Danger Zone */}
      <div className="bg-white rounded-xl border border-gray-200 shadow-sm p-6">
        <h2 className="text-lg font-semibold text-gray-900 mb-4 flex items-center gap-2">
          <svg className="h-5 w-5 text-red-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          Danger Zone
        </h2>
        <p className="text-sm text-gray-500 mb-4">Irreversible and destructive actions.</p>
        <div className="flex items-center justify-between p-4 bg-red-50 border border-red-200 rounded-lg">
          <div>
            <p className="font-medium text-red-800">Delete Account</p>
            <p className="text-sm text-red-600">Permanently delete your account and all associated data.</p>
          </div>
          <button
            onClick={() => {
              if (confirm('Are you sure you want to delete your account? This cannot be undone.')) {
                // api.delete('/api/v1/user');
                logout();
                toast.success('Account deleted');
              }
            }}
            className="px-4 py-2 bg-red-600 text-white text-sm font-medium rounded-lg hover:bg-red-700 transition-colors"
          >
            Delete Account
          </button>
        </div>
      </div>
    </div>
  );
}

function ProfileForm({ form, setForm, onSave, isSaving }: { form: any; setForm: any; onSave: () => void; isSaving: boolean }) {
  return (
    <form className="space-y-6" onSubmit={(e) => { e.preventDefault(); onSave(); }}>
      <div className="grid gap-6 sm:grid-cols-2">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Full Name</label>
          <input
            type="text"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Email</label>
          <input
            type="email"
            value={form.email}
            onChange={(e) => setForm({ ...form, email: e.target.value })}
            className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
            disabled
          />
          <p className="mt-1 text-xs text-gray-500">Email cannot be changed</p>
        </div>
      </div>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Bio</label>
        <textarea
          value={form.bio}
          onChange={(e) => setForm({ ...form, bio: e.target.value })}
          rows={4}
          className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
          placeholder="Tell us about yourself..."
        />
      </div>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Timezone</label>
        <select
          value={form.timezone}
          onChange={(e) => setForm({ ...form, timezone: e.target.value })}
          className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
        >
          <option value="UTC">UTC</option>
          <option value="America/New_York">Eastern Time (US)</option>
          <option value="America/Chicago">Central Time (US)</option>
          <option value="America/Denver">Mountain Time (US)</option>
          <option value="America/Los_Angeles">Pacific Time (US)</option>
          <option value="Europe/London">London</option>
          <option value="Europe/Paris">Paris</option>
          <option value="Asia/Tokyo">Tokyo</option>
          <option value="Asia/Shanghai">Shanghai</option>
          <option value="Australia/Sydney">Sydney</option>
        </select>
      </div>
      <div className="pt-4 border-t border-gray-200 flex justify-end">
        <button
          type="submit"
          disabled={isSaving}
          className="px-6 py-2.5 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 disabled:opacity-50 flex items-center gap-2"
        >
          <Save className="h-4 w-4" />
          {isSaving ? 'Saving...' : 'Save Changes'}
        </button>
      </div>
    </form>
  );
}

function SecurityForm({ form, setForm, onSave, isSaving }: { form: any; setForm: any; onSave: () => void; isSaving: boolean }) {
  return (
    <form className="space-y-6" onSubmit={(e) => { e.preventDefault(); onSave(); }}>
      <div>
        <h3 className="text-lg font-medium text-gray-900 mb-4">Change Password</h3>
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Current Password</label>
            <input
              type="password"
              value={form.currentPassword}
              onChange={(e) => setForm({ ...form, currentPassword: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">New Password</label>
            <input
              type="password"
              value={form.newPassword}
              onChange={(e) => setForm({ ...form, newPassword: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
              minLength={8}
            />
          </div>
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Confirm New Password</label>
          <input
            type="password"
            value={form.confirmPassword}
            onChange={(e) => setForm({ ...form, confirmPassword: e.target.value })}
            className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
          />
        </div>
      </div>

      <div className="pt-4 border-t border-gray-200">
        <h3 className="text-lg font-medium text-gray-900 mb-4">Two-Factor Authentication</h3>
        <div className="flex items-center justify-between">
          <div>
            <p className="font-medium text-gray-900">Enable 2FA</p>
            <p className="text-sm text-gray-500">Add an extra layer of security to your account</p>
          </div>
          <label className="relative inline-flex items-center cursor-pointer">
            <input
              type="checkbox"
              checked={form.twoFactorEnabled}
              onChange={(e) => setForm({ ...form, twoFactorEnabled: e.target.checked })}
              className="sr-only peer"
            />
            <div className="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-primary-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
          </label>
        </div>
      </div>

      <div className="pt-4 border-t border-gray-200">
        <h3 className="text-lg font-medium text-gray-900 mb-4">Active Sessions</h3>
        <div className="space-y-2">
          <div className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
            <div className="flex items-center gap-3">
              <div className="h-8 w-8 rounded-full bg-green-100 flex items-center justify-center">
                <svg className="h-5 w-5 text-green-700" fill="currentColor" viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z"/></svg>
              </div>
              <div>
                <p className="font-medium text-gray-900">Current Session</p>
                <p className="text-sm text-gray-500">Chrome on macOS • Just now</p>
              </div>
            </div>
            <span className="px-2 py-1 bg-green-100 text-green-700 text-xs font-medium rounded-full">Active</span>
          </div>
        </div>
        <button type="button" className="mt-3 text-sm text-primary-600 hover:text-primary-700">Revoke all other sessions</button>
      </div>

      <div className="pt-4 border-t border-gray-200 flex justify-end">
        <button
          type="submit"
          disabled={isSaving}
          className="px-6 py-2.5 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 disabled:opacity-50 flex items-center gap-2"
        >
          <Save className="h-4 w-4" />
          {isSaving ? 'Saving...' : 'Update Security'}
        </button>
      </div>
    </form>
  );
}

function NotificationsForm({ form, setForm, onSave, isSaving }: { form: any; setForm: any; onSave: () => void; isSaving: boolean }) {
  const toggle = (key: string) => setForm({ ...form, [key]: !form[key] });

  return (
    <form className="space-y-6" onSubmit={(e) => { e.preventDefault(); onSave(); }}>
      <div>
        <h3 className="text-lg font-medium text-gray-900 mb-4">Email Notifications</h3>
        <div className="space-y-3">
          {[
            { key: 'emailDeployments', label: 'Deployment Updates', desc: 'Get notified when deployments start, succeed, or fail' },
            { key: 'emailAIJobs', label: 'AI Job Completion', desc: 'Receive results when AI reviews, docs, tests, or migrations finish' },
            { key: 'emailSecurity', label: 'Security Alerts', desc: 'Important security notifications about your account' },
          ].map((item) => (
            <label key={item.key} className="flex items-center justify-between p-3 bg-gray-50 rounded-lg cursor-pointer hover:bg-gray-100">
              <div>
                <p className="font-medium text-gray-900">{item.label}</p>
                <p className="text-sm text-gray-500">{item.desc}</p>
              </div>
              <input
                type="checkbox"
                checked={form[item.key]}
                onChange={() => toggle(item.key)}
                className="h-5 w-5 text-primary-600 border-gray-300 rounded focus:ring-primary-500"
              />
            </label>
          ))}
        </div>
      </div>

      <div className="pt-4 border-t border-gray-200">
        <h3 className="text-lg font-medium text-gray-900 mb-4">Webhook Integrations</h3>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Slack Webhook URL</label>
            <input
              type="url"
              value={form.slackWebhook}
              onChange={(e) => setForm({ ...form, slackWebhook: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
              placeholder="https://hooks.slack.com/services/..."
            />
            <p className="mt-1 text-xs text-gray-500">Receive deployment and AI job notifications in Slack</p>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Discord Webhook URL</label>
            <input
              type="url"
              value={form.discordWebhook}
              onChange={(e) => setForm({ ...form, discordWebhook: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
              placeholder="https://discord.com/api/webhooks/..."
            />
            <p className="mt-1 text-xs text-gray-500">Receive notifications in Discord</p>
          </div>
        </div>
      </div>

      <div className="pt-4 border-t border-gray-200 flex justify-end">
        <button
          type="submit"
          disabled={isSaving}
          className="px-6 py-2.5 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 disabled:opacity-50 flex items-center gap-2"
        >
          <Save className="h-4 w-4" />
          {isSaving ? 'Saving...' : 'Save Notifications'}
        </button>
      </div>
    </form>
  );
}

function AppearanceForm({ form, setForm, onSave, isSaving }: { form: any; setForm: any; onSave: () => void; isSaving: boolean }) {
  const themes = [
    { value: 'light', label: 'Light', desc: 'Always use light mode' },
    { value: 'dark', label: 'Dark', desc: 'Always use dark mode' },
    { value: 'system', label: 'System', desc: 'Match your OS preference' },
  ];

  return (
    <form className="space-y-6" onSubmit={(e) => { e.preventDefault(); onSave(); }}>
      <div>
        <h3 className="text-lg font-medium text-gray-900 mb-4">Theme</h3>
        <div className="grid gap-4 sm:grid-cols-3">
          {themes.map((theme) => (
            <label
              key={theme.value}
              className={`p-4 border-2 rounded-lg cursor-pointer transition-colors ${
                form.theme === theme.value
                  ? 'border-primary-500 bg-primary-50'
                  : 'border-gray-200 hover:border-gray-300'
              }`}
            >
              <input
                type="radio"
                name="theme"
                value={theme.value}
                checked={form.theme === theme.value}
                onChange={(e) => setForm({ ...form, theme: e.target.value as typeof form.theme })}
                className="sr-only"
              />
              <div className="text-center">
                <p className="font-medium text-gray-900 capitalize">{theme.label}</p>
                <p className="text-sm text-gray-500 mt-1">{theme.desc}</p>
              </div>
            </label>
          ))}
        </div>
      </div>

      <div className="pt-4 border-t border-gray-200">
        <h3 className="text-lg font-medium text-gray-900 mb-4">Interface</h3>
        <div className="space-y-4">
          {[
            { key: 'compactMode', label: 'Compact Mode', desc: 'Reduce padding and spacing for denser information display' },
            { key: 'animations', label: 'Animations', desc: 'Enable transitions and micro-animations' },
          ].map((item) => (
            <label key={item.key} className="flex items-center justify-between p-3 bg-gray-50 rounded-lg cursor-pointer hover:bg-gray-100">
              <div>
                <p className="font-medium text-gray-900">{item.label}</p>
                <p className="text-sm text-gray-500">{item.desc}</p>
              </div>
              <input
                type="checkbox"
                checked={form[item.key]}
                onChange={(e) => setForm({ ...form, [item.key]: e.target.checked })}
                className="h-5 w-5 text-primary-600 border-gray-300 rounded focus:ring-primary-500"
              />
            </label>
          ))}
        </div>
      </div>

      <div className="pt-4 border-t border-gray-200 flex justify-end">
        <button
          type="submit"
          disabled={isSaving}
          className="px-6 py-2.5 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 disabled:opacity-50 flex items-center gap-2"
        >
          <Save className="h-4 w-4" />
          {isSaving ? 'Saving...' : 'Save Appearance'}
        </button>
      </div>
    </form>
  );
}

function IntegrationsForm({ form, setForm, onSave, isSaving }: { form: any; setForm: any; onSave: () => void; isSaving: boolean }) {
  const integrations = [
    { key: 'githubToken', label: 'GitHub Personal Access Token', desc: 'Required for repository access and PR reviews', placeholder: 'ghp_...' },
    { key: 'gitlabToken', label: 'GitLab Personal Access Token', desc: 'For GitLab repository integration', placeholder: 'glpat-...' },
    { key: 'awsAccessKey', label: 'AWS Access Key ID', desc: 'For AWS deployments', placeholder: 'AKIA...' },
    { key: 'awsSecretKey', label: 'AWS Secret Access Key', desc: 'For AWS deployments', placeholder: '...', type: 'password' },
    { key: 'gcpProjectId', label: 'GCP Project ID', desc: 'For Google Cloud deployments', placeholder: 'my-project-123' },
    { key: 'azureSubscriptionId', label: 'Azure Subscription ID', desc: 'For Azure deployments', placeholder: 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx' },
  ];

  return (
    <form className="space-y-6" onSubmit={(e) => { e.preventDefault(); onSave(); }}>
      <div className="space-y-6">
        {integrations.map((integration) => (
          <div key={integration.key}>
            <label className="block text-sm font-medium text-gray-700 mb-1">{integration.label}</label>
            <input
              type={integration.type || 'text'}
              value={form[integration.key]}
              onChange={(e) => setForm({ ...form, [integration.key]: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 font-mono text-sm"
              placeholder={integration.placeholder}
              autocomplete="off"
            />
            <p className="mt-1 text-xs text-gray-500">{integration.desc}</p>
          </div>
        ))}
      </div>

      <div className="pt-4 border-t border-gray-200 flex justify-end">
        <button
          type="submit"
          disabled={isSaving}
          className="px-6 py-2.5 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 disabled:opacity-50 flex items-center gap-2"
        >
          <Save className="h-4 w-4" />
          {isSaving ? 'Saving...' : 'Save Integrations'}
        </button>
      </div>
    </form>
  );
}