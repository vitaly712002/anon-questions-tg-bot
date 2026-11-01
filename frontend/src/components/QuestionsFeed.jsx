import React, { useState } from 'react';
import { MessageSquare, Mic, Video, Image, CheckCircle, Clock, Search, ArrowRight, User } from 'lucide-react';
import { getMediaUrl } from '../api';

export default function QuestionsFeed({ questions, loading, onRefresh }) {
  const [filterType, setFilterType] = useState('all');
  const [search, setSearch] = useState('');

  const filtered = (questions || []).filter((q) => {
    if (filterType !== 'all' && q.content_type !== filterType) return false;
    if (search.trim()) {
      const s = search.toLowerCase();
      const textMatch = q.text?.toLowerCase().includes(s);
      const answerMatch = q.answer_text?.toLowerCase().includes(s);
      const recipientMatch = q.recipient_username?.toLowerCase().includes(s);
      const senderMatch = q.sender_username?.toLowerCase().includes(s);
      return textMatch || answerMatch || recipientMatch || senderMatch;
    }
    return true;
  });

  const typeIcons = {
    text: MessageSquare,
    voice: Mic,
    video_note: Video,
    video: Video,
    photo: Image,
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-white">Лента вопросов (Live Feed)</h2>
          <p className="text-sm text-slate-400">
            Вопросы расшифровываются ключом безопасности на лету
          </p>
        </div>
        <button
          onClick={onRefresh}
          className="self-start sm:self-auto px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 text-sm font-medium rounded-xl border border-slate-700 transition-colors"
        >
          Обновить
        </button>
      </div>

      {/* Фильтры и поиск */}
      <div className="flex flex-col sm:flex-row gap-3">
        <div className="relative flex-1">
          <Search className="w-4 h-4 absolute left-3.5 top-3 text-slate-500" />
          <input
            type="text"
            placeholder="Поиск по тексту или юзернейму..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-10 pr-4 py-2 bg-slate-900/60 border border-slate-800 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:border-indigo-500"
          />
        </div>

        <div className="flex space-x-1 overflow-x-auto pb-1">
          {[
            { id: 'all', label: 'Все' },
            { id: 'text', label: 'Текст' },
            { id: 'voice', label: 'Голосовые' },
            { id: 'video_note', label: 'Кружочки' },
            { id: 'photo', label: 'Фото' },
          ].map((t) => (
            <button
              key={t.id}
              onClick={() => setFilterType(t.id)}
              className={`px-3 py-1.5 rounded-lg text-xs font-medium whitespace-nowrap transition-all ${
                filterType === t.id
                  ? 'bg-indigo-600 text-white shadow'
                  : 'bg-slate-900 border border-slate-800 text-slate-400 hover:text-white'
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>
      </div>

      {/* Список карточек */}
      {loading ? (
        <div className="p-12 text-center text-slate-500">Загрузка вопросов...</div>
      ) : filtered.length === 0 ? (
        <div className="p-12 bg-slate-900/30 border border-slate-800 rounded-2xl text-center text-slate-500">
          Вопросы не найдены
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4">
          {filtered.map((q) => {
            const Icon = typeIcons[q.content_type] || MessageSquare;
            const dateStr = new Date(q.created_at).toLocaleString('ru-RU', {
              day: 'numeric',
              month: 'short',
              hour: '2-digit',
              minute: '2-digit',
            });

            return (
              <div
                key={q.id}
                className="bg-slate-900/70 border border-slate-800 hover:border-slate-700/80 rounded-2xl p-5 transition-all shadow-sm"
              >
                <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-800/80 pb-3 mb-3">
                  <div className="flex items-center space-x-2 text-sm">
                    {/* Отправитель (расшифрован) */}
                    <div className="flex items-center space-x-1.5 text-amber-400 bg-amber-500/10 px-2.5 py-1 rounded-lg border border-amber-500/20 font-medium">
                      <User className="w-3.5 h-3.5" />
                      <span>{q.sender_username || 'Аноним'}</span>
                    </div>

                    <ArrowRight className="w-3.5 h-3.5 text-slate-600" />

                    {/* Получатель */}
                    <div className="flex items-center space-x-1.5 text-indigo-400 bg-indigo-500/10 px-2.5 py-1 rounded-lg border border-indigo-500/20 font-medium">
                      <span>@{q.recipient_username || q.recipient_name || 'id' + q.recipient_tg_id}</span>
                    </div>
                  </div>

                  <div className="flex items-center space-x-3 text-xs text-slate-400">
                    <span className="flex items-center space-x-1">
                      <Icon className="w-3.5 h-3.5 text-slate-500" />
                      <span className="capitalize">{q.content_type}</span>
                    </span>
                    <span>•</span>
                    <span>{dateStr}</span>
                    <span>•</span>
                    {q.answered ? (
                      <span className="flex items-center space-x-1 text-emerald-400">
                        <CheckCircle className="w-3.5 h-3.5" />
                        <span>Отвечен</span>
                      </span>
                    ) : (
                      <span className="flex items-center space-x-1 text-slate-500">
                        <Clock className="w-3.5 h-3.5" />
                        <span>Ожидает</span>
                      </span>
                    )}
                  </div>
                </div>

                {/* Содержимое вопроса */}
                <div className="mt-2">
                  {/* Текст */}
                  {q.text && (
                    <p className="text-slate-100 text-sm leading-relaxed whitespace-pre-wrap bg-slate-950/40 p-3 rounded-xl border border-slate-800/50">
                      {q.text}
                    </p>
                  )}

                  {/* Медиафайлы через прокси Telegram */}
                  {q.file_id && (
                    <div className="mt-3">
                      {q.content_type === 'voice' && (
                        <div className="bg-slate-950/60 p-3 rounded-xl border border-slate-800 flex items-center space-x-3">
                          <Mic className="w-5 h-5 text-indigo-400 shrink-0" />
                          <audio
                            controls
                            src={getMediaUrl(q.file_id)}
                            className="w-full h-8"
                          />
                        </div>
                      )}

                      {q.content_type === 'video_note' && (
                        <div className="flex items-center space-x-4">
                          <div className="w-48 h-48 rounded-full overflow-hidden border-2 border-indigo-500/50 shadow-lg bg-black">
                            <video
                              controls
                              loop
                              src={getMediaUrl(q.file_id)}
                              className="w-full h-full object-cover"
                            />
                          </div>
                          <span className="text-xs text-slate-500">Видеосообщение («кружочек»)</span>
                        </div>
                      )}

                      {q.content_type === 'photo' && (
                        <div className="max-w-xs rounded-xl overflow-hidden border border-slate-800 bg-black">
                          <img
                            src={getMediaUrl(q.file_id)}
                            alt="Фото вопроса"
                            className="w-full h-auto object-contain hover:scale-105 transition-transform"
                          />
                        </div>
                      )}

                      {q.content_type === 'video' && (
                        <div className="max-w-md rounded-xl overflow-hidden border border-slate-800 bg-black">
                          <video
                            controls
                            src={getMediaUrl(q.file_id)}
                            className="w-full h-auto"
                          />
                        </div>
                      )}
                    </div>
                  )}
                </div>

                {/* Содержимое ответа (если вопрос отвечен) */}
                {q.answered && (
                  <div className="mt-4 pt-3 border-t border-slate-800/80">
                    <div className="flex items-center gap-1.5 text-xs font-semibold text-emerald-400 mb-2">
                      <MessageSquare className="w-3.5 h-3.5" />
                      <span>Ответ получателя (@{q.recipient_username || q.recipient_name || 'пользователь'}):</span>
                      {q.answered_at && (
                        <span className="text-[11px] font-normal text-slate-500 ml-1">
                          • {new Date(q.answered_at).toLocaleString('ru-RU', {
                              day: 'numeric',
                              month: 'short',
                              hour: '2-digit',
                              minute: '2-digit',
                            })}
                        </span>
                      )}
                    </div>

                    {q.answer_text ? (
                      <div className="p-3 bg-emerald-950/20 border border-emerald-500/20 rounded-xl text-sm text-emerald-100 whitespace-pre-wrap leading-relaxed shadow-sm">
                        {q.answer_text}
                      </div>
                    ) : q.answer_media_url ? (
                      <div className="p-3 bg-slate-950/60 border border-slate-800 rounded-xl">
                        <span className="text-xs text-slate-400">Медиа-ответ:</span>
                        <img
                          src={q.answer_media_url}
                          alt="Медиа-ответ"
                          className="mt-2 max-w-xs rounded-lg border border-slate-800"
                        />
                      </div>
                    ) : (
                      <div className="p-2.5 bg-slate-950/30 border border-slate-800/50 rounded-xl text-xs text-slate-400 italic">
                        Ответ был отправлен в Telegram (текст не сохранён для старых сообщений).
                      </div>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
