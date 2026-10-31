import React, { useState, useEffect } from 'react';
import { Save, AlertTriangle, Check, RefreshCw } from 'lucide-react';
import { getSettings, updateSettings } from '../api';

export default function SettingsPanel() {
  const [settings, setSettings] = useState({
    rate_burst_per_hour: '5',
    rate_daily_limit: '20',
    maintenance_mode: 'false',
    welcome_message: '',
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [savedSuccess, setSavedSuccess] = useState(false);

  const fetchSettings = async () => {
    try {
      setLoading(true);
      const data = await getSettings();
      setSettings((prev) => ({ ...prev, ...data }));
    } catch (err) {
      console.error('Ошибка загрузки настроек', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSettings();
  }, []);

  const handleSave = async (e) => {
    e.preventDefault();
    try {
      setSaving(true);
      await updateSettings(settings);
      setSavedSuccess(true);
      setTimeout(() => setSavedSuccess(false), 3000);
    } catch (err) {
      alert('Ошибка сохранения: ' + err.message);
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <div className="p-12 text-center text-slate-500">Загрузка параметров бота...</div>;
  }

  const isMaintenance = settings.maintenance_mode === 'true';

  return (
    <div className="max-w-3xl space-y-6">
      <div>
        <h2 className="text-xl font-bold text-white">Конфигурация бота через UI</h2>
        <p className="text-sm text-slate-400">
          Изменения сохраняются в базе данных и применяются на лету без перезагрузки сервера
        </p>
      </div>

      {isMaintenance && (
        <div className="bg-amber-500/10 border border-amber-500/30 rounded-2xl p-4 flex items-center space-x-3 text-amber-400">
          <AlertTriangle className="w-5 h-5 shrink-0" />
          <div className="text-sm">
            <span className="font-semibold">Включен режим техобслуживания!</span> Бот временно
            отклоняет новые вопросы и предупреждает пользователей об обновлении сервиса.
          </div>
        </div>
      )}

      <form onSubmit={handleSave} className="bg-slate-900/70 border border-slate-800 rounded-2xl p-6 space-y-6 shadow-sm">
        {/* Режим техобслуживания */}
        <div className="flex items-center justify-between pb-6 border-b border-slate-800">
          <div>
            <label className="text-sm font-semibold text-white block">Режим техобслуживания (Maintenance Mode)</label>
            <p className="text-xs text-slate-400 mt-0.5">Временно приостанавливает приём вопросов</p>
          </div>
          <button
            type="button"
            onClick={() =>
              setSettings((prev) => ({
                ...prev,
                maintenance_mode: prev.maintenance_mode === 'true' ? 'false' : 'true',
              }))
            }
            className={`w-12 h-6 flex items-center rounded-full p-1 transition-colors ${
              isMaintenance ? 'bg-amber-500' : 'bg-slate-700'
            }`}
          >
            <div
              className={`bg-white w-4 h-4 rounded-full shadow-md transform transition-transform ${
                isMaintenance ? 'translate-x-6' : 'translate-x-0'
              }`}
            />
          </button>
        </div>

        {/* Лимиты антифлуда */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pb-6 border-b border-slate-800">
          <div>
            <label className="text-xs font-semibold text-slate-300 block mb-1.5">
              Лимит вопросов в час (Burst)
            </label>
            <input
              type="number"
              min="1"
              max="100"
              value={settings.rate_burst_per_hour || '5'}
              onChange={(e) => setSettings({ ...settings, rate_burst_per_hour: e.target.value })}
              className="w-full px-3.5 py-2 bg-slate-950 border border-slate-800 rounded-xl text-sm text-white focus:outline-none focus:border-indigo-500"
            />
            <p className="text-xs text-slate-500 mt-1">Максимум вопросов подряд от одного юзера</p>
          </div>

          <div>
            <label className="text-xs font-semibold text-slate-300 block mb-1.5">
              Суточный лимит вопросов
            </label>
            <input
              type="number"
              min="1"
              max="500"
              value={settings.rate_daily_limit || '20'}
              onChange={(e) => setSettings({ ...settings, rate_daily_limit: e.target.value })}
              className="w-full px-3.5 py-2 bg-slate-950 border border-slate-800 rounded-xl text-sm text-white focus:outline-none focus:border-indigo-500"
            />
            <p className="text-xs text-slate-500 mt-1">Лимит вопросов за последние 24 часа</p>
          </div>
        </div>

        {/* Приветственный текст */}
        <div>
          <label className="text-xs font-semibold text-slate-300 block mb-1.5">
            Приветственное сообщение /start
          </label>
          <textarea
            rows="3"
            value={settings.welcome_message || ''}
            onChange={(e) => setSettings({ ...settings, welcome_message: e.target.value })}
            className="w-full px-3.5 py-2 bg-slate-950 border border-slate-800 rounded-xl text-sm text-white focus:outline-none focus:border-indigo-500"
            placeholder="Текст при старте..."
          />
        </div>

        {/* Кнопка сохранения */}
        <div className="flex items-center justify-between pt-2">
          {savedSuccess ? (
            <span className="inline-flex items-center space-x-1.5 text-xs text-emerald-400 font-medium">
              <Check className="w-4 h-4" />
              <span>Настройки успешно сохранены!</span>
            </span>
          ) : (
            <span />
          )}

          <button
            type="submit"
            disabled={saving}
            className="flex items-center space-x-2 px-5 py-2.5 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm font-semibold rounded-xl shadow-lg shadow-indigo-500/20 transition-all"
          >
            {saving ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
            <span>{saving ? 'Сохранение...' : 'Сохранить настройки'}</span>
          </button>
        </div>
      </form>
    </div>
  );
}
