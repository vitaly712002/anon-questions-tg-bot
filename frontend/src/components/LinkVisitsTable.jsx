import React, { useState } from 'react';
import { Search, Link2, ExternalLink, ArrowRight, User, Clock } from 'lucide-react';

export default function LinkVisitsTable({ visits, loading, onRefresh }) {
  const [search, setSearch] = useState('');

  const filtered = (visits || []).filter((v) => {
    if (!search.trim()) return true;
    const s = search.toLowerCase();
    return (
      v.guest_username?.toLowerCase().includes(s) ||
      v.guest_name?.toLowerCase().includes(s) ||
      String(v.guest_tg_id).includes(s) ||
      v.recipient_username?.toLowerCase().includes(s) ||
      v.recipient_name?.toLowerCase().includes(s) ||
      String(v.recipient_tg_id).includes(s) ||
      v.link_alias?.toLowerCase().includes(s) ||
      v.link_token?.toLowerCase().includes(s)
    );
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-white">Переходы по ссылкам</h2>
          <p className="text-sm text-slate-400">
            Кто и к кому переходил по персональным ссылкам бота
          </p>
        </div>
        <button
          onClick={onRefresh}
          className="self-start sm:self-auto px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 text-sm font-medium rounded-xl border border-slate-700 transition-colors"
        >
          Обновить
        </button>
      </div>

      <div className="relative">
        <Search className="w-4 h-4 absolute left-3.5 top-3 text-slate-500" />
        <input
          type="text"
          placeholder="Поиск по гостю, получателю, юзернейму, ID или алиасу ссылки..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full pl-10 pr-4 py-2 bg-slate-900/60 border border-slate-800 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:border-indigo-500"
        />
      </div>

      <div className="bg-slate-900/70 border border-slate-800 rounded-2xl overflow-hidden shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm text-slate-300">
            <thead className="bg-slate-950/60 text-xs uppercase text-slate-400 border-b border-slate-800">
              <tr>
                <th className="px-6 py-4">Кто перешёл (Гость)</th>
                <th className="px-6 py-4 text-center">➔</th>
                <th className="px-6 py-4">Кому (Владелец ссылки)</th>
                <th className="px-6 py-4">Ссылка / Алиас</th>
                <th className="px-6 py-4 text-right">Дата и время</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/60">
              {loading ? (
                <tr>
                  <td colSpan="5" className="px-6 py-12 text-center text-slate-500">
                    Загрузка переходов...
                  </td>
                </tr>
              ) : filtered.length === 0 ? (
                <tr>
                  <td colSpan="5" className="px-6 py-12 text-center text-slate-500">
                    {search ? 'Переходы не найдены по запросу' : 'Пока нет зафиксированных переходов'}
                  </td>
                </tr>
              ) : (
                filtered.map((v) => {
                  const linkDisplay = v.link_alias ? `@${v.link_alias}` : (v.link_token || '—');
                  const dateStr = new Date(v.created_at).toLocaleString('ru-RU', {
                    day: '2-digit',
                    month: '2-digit',
                    year: 'numeric',
                    hour: '2-digit',
                    minute: '2-digit',
                    second: '2-digit',
                  });

                  return (
                    <tr key={v.id} className="hover:bg-slate-800/30 transition-colors">
                      {/* Гость */}
                      <td className="px-6 py-4">
                        <div className="flex items-center space-x-3">
                          <div className="w-9 h-9 rounded-lg bg-amber-500/10 border border-amber-500/20 flex items-center justify-center text-amber-400 font-medium">
                            <User className="w-4 h-4" />
                          </div>
                          <div>
                            <div className="font-medium text-white">
                              {v.guest_name || 'Анонимный пользователь'}
                            </div>
                            <div className="text-xs text-amber-400/90">
                              {v.guest_username ? `@${v.guest_username}` : 'без юзернейма'}
                              <span className="text-slate-500 ml-1.5 font-mono">
                                id:{v.guest_tg_id}
                              </span>
                            </div>
                          </div>
                        </div>
                      </td>

                      {/* Стрелка */}
                      <td className="px-6 py-4 text-center text-slate-500">
                        <ArrowRight className="w-4 h-4 mx-auto text-indigo-400/60" />
                      </td>

                      {/* Получатель */}
                      <td className="px-6 py-4">
                        <div className="flex items-center space-x-3">
                          <div className="w-9 h-9 rounded-lg bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400 font-medium">
                            <User className="w-4 h-4" />
                          </div>
                          <div>
                            <div className="font-medium text-white">
                              {v.recipient_name || 'Владелец ссылки'}
                            </div>
                            <div className="text-xs text-indigo-400/90">
                              {v.recipient_username ? `@${v.recipient_username}` : 'без юзернейма'}
                              <span className="text-slate-500 ml-1.5 font-mono">
                                id:{v.recipient_tg_id}
                              </span>
                            </div>
                          </div>
                        </div>
                      </td>

                      {/* Ссылка */}
                      <td className="px-6 py-4">
                        <span className="inline-flex items-center px-2.5 py-1 rounded-lg text-xs font-mono bg-slate-800 text-slate-200 border border-slate-700">
                          <Link2 className="w-3.5 h-3.5 mr-1 text-slate-400" />
                          {linkDisplay}
                        </span>
                      </td>

                      {/* Дата */}
                      <td className="px-6 py-4 text-right text-xs text-slate-400 font-mono whitespace-nowrap">
                        <div className="flex items-center justify-end space-x-1.5">
                          <Clock className="w-3.5 h-3.5 text-slate-500" />
                          <span>{dateStr}</span>
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
