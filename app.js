// 1. Генерация случайных аккордов
function generateChords() {
    const key = document.getElementById('chordKey').value;
    const level = document.getElementById('chordLevel').value;
    const length = document.getElementById('chordLength').value;

    fetch('/api/chords/random?key=' + key + '&level=' + level + '&length=' + length)
        .then(res => res.json())
        .then(data => {
            const box = document.getElementById('chordsResult');
            box.innerHTML = '';
            data.sequence.forEach(chord => {
                box.innerHTML += '<div class="chord-box">' + chord + '</div>';
            });
        });
}

// 2. Генератор ритмического рисунка (боя)
const strumPatterns = {
    "44": [
        "⬇️ ⬇️ ⬆️ ⬆️ ⬇️ ⬆️ (Шестерка)",
        "⬇️ ❌ ⬆️ ⬆️ ❌ ⬆️ (Шестерка с глушением)",
        "⬇️ ⬇️ ⬆️ ⬇️ ⬆️ (Пятерка)",
        "⬇️ ❌ ⬆️ ⬇️ ❌ ⬆️ (Четверка)",
        "⬇️ ⬇️ ⬇️ ⬇️ (Простые четверти)"
    ],
    "34": [
        "⬇️ ⬆️ ⬆️ (Вальсовый простой)",
        "⬇️ ⬇️ ⬆️ ⬇️ ⬆️ (Испанский вальс)",
        "⬇️ ❌ ❌ (Вальс с глушением)"
    ]
};

function generateStrum() {
    const type = document.getElementById('strumType').value;
    let pool = [];

    if (type === "all") {
        pool = [...strumPatterns["44"], ...strumPatterns["34"]];
    } else {
        pool = strumPatterns[type];
    }

    const randomIndex = Math.floor(Math.random() * pool.length);
    document.getElementById('strumResult').innerText = pool[randomIndex];
}

// 3. Аудио-метроном (Web Audio API)
let audioCtx = null;
let isPlaying = false;
let metronomeInterval = null;

function toggleMetronome() {
    const btn = document.getElementById('metronomeBtn');
    
    if (isPlaying) {
        clearInterval(metronomeInterval);
        isPlaying = false;
        btn.innerText = 'Старт';
        btn.style.background = '#00adb5';
        return;
    }

    const bpm = parseInt(document.getElementById('bpmInput').value) || 120;
    const intervalMs = (60 / bpm) * 1000;

    if (!audioCtx) {
        audioCtx = new (window.AudioContext || window.webkitAudioContext)();
    }

    isPlaying = true;
    btn.innerText = 'Стоп';
    btn.style.background = '#ff4d4d';

    playClick();
    metronomeInterval = setInterval(playClick, intervalMs);
}

function playClick() {
    if (!audioCtx) return;
    
    const osc = audioCtx.createOscillator();
    const gainNode = audioCtx.createGain();

    osc.connect(gainNode);
    gainNode.connect(audioCtx.destination);

    osc.type = 'sine';
    osc.frequency.setValueAtTime(800, audioCtx.currentTime); 
    
    gainNode.gain.setValueAtTime(1, audioCtx.currentTime);
    gainNode.gain.exponentialRampToValueAtTime(0.001, audioCtx.currentTime + 0.05);

    osc.start(audioCtx.currentTime);
    osc.stop(audioCtx.currentTime + 0.05);
}

// 4. Поиск сохраненных треков в каталоге
function searchSongs() {
    const q = document.getElementById('searchInput').value;
    fetch('/api/tabs?search=' + encodeURIComponent(q))
        .then(res => res.json())
        .then(data => {
            const box = document.getElementById('songsResult');
            box.innerHTML = '';
            if(!data || data.length === 0) {
                box.innerHTML = '<p style="color: #a8a8b3;">Ничего не найдено</p>';
                return;
            }
            data.forEach(song => {
                let innerContent = '';
                try {
                    // Если внутри песни лежит JSON-анализ
                    const trackMeta = JSON.parse(song.content);
                    innerContent = `<p style="color: #00adb5; font-weight: bold;">[AI Разбор аудиофайла]</p><br>`;
                    trackMeta.chords_vector.forEach(item => {
                        innerContent += `<b>[${item.timeStr}]</b> Акаорды: ${item.chord1} — ${item.chord2} (Пик: ${item.peak})<br>`;
                    });
                } catch(e) {
                    // Если это старая текстовая песня
                    innerContent = `<pre>${song.content}</pre>`;
                }

                box.innerHTML += `
                    <div class="song-card" style="margin-bottom: 15px;">
                        <h3>${song.title}</h3>
                        <h4>${song.artist_name}</h4>
                        <div style="margin-top: 10px; background: #121214; padding: 15px; border-radius: 6px; color: #ffb64d; line-height: 1.6;">
                            ${innerContent}
                        </div>
                    </div>
                `;
            });
        });
}


// 5. Загрузка аудиофайла и AI распознавание аккордов
// Карта популярнейших аппликатур (0 - открытая струна, х - глушить, цифра - номер лада от 1 до 6 струны)
const chordLibrarySVG = {
    "Am": "🏽 x 0 2 2 1 0",
    "C":  "🏽 x 3 2 0 1 0",
    "Em": "🏽 0 2 2 0 0 0",
    "G":  "🏽 3 2 0 0 0 3",
    "Dm": "🏽 x x 0 2 3 1",
    "F":  "🏽 1 3 3 2 1 1 (Баррэ)",
    "D":  "🏽 x x 0 2 3 2",
    "E":  "🏽 0 2 2 1 0 0"
};

function uploadAndRecognizeAudio() {
    const artist = document.getElementById('audioArtist').value;
    const title = document.getElementById('audioTitle').value;
    const fileInput = document.getElementById('audioFile');

    if (!artist || !title || fileInput.files.length === 0) {
        alert('Пожалуйста, заполните поля и выберите аудиофайл!');
        return;
    }

    const statusBox = document.getElementById('aiStatus');
    statusBox.innerText = '⏳ Парсим оригинальный текст и строим интерактивную сетку аккордов...';

    const file = fileInput.files[0];
    const audioUrl = URL.createObjectURL(file);
    const formData = new FormData();
    formData.append('artist_name', artist);
    formData.append('title', title);
    formData.append('audio_file', file);
    formData.append('duration_sec', 180); // базовый тайминг

    fetch('/api/tabs/upload-audio', {
        method: 'POST',
        body: formData
    })
    .then(res => res.json())
    .then(data => {
        if (data.error) {
            alert('Ошибка: ' + data.error);
            statusBox.innerText = '';
        } else {
            statusBox.innerText = '✅ Текст песни и аппликатуры успешно загружены!';
            
            const trackMeta = JSON.parse(data.content);
            
            // Превращаем обычные текстовые аккорды [Am] в парящие HTML-плашки
            let formattedLyrics = trackMeta.text_content
                .replace(/\[([A-G][b#]?(m|maj|min|strings)?\d*)\]/g, '<span class="chord-wrapper" data-chord="$1"></span>');

            let htmlCard = `
                <div class="song-card">
                    <h3>${data.title}</h3>
                    <h4>${data.artist_name}</h4>
                    
                    <audio id="mainAudioComponent" controls style="width: 100%; margin: 15px 0;"></audio>
                    
                    <!-- Разбор песни с парящими аккордами над строками -->
                    <div class="lyrics-container">
                        ${formattedLyrics}
                    </div>

                    <!-- Раздел аппликатур (схем ладов) в самом низу песни -->
                    <h4 style="margin-top: 30px; color: #fff; font-size: 1.2rem;">📌 Аппликатуры аккордов песни:</h4>
                    <div class="chords-diagrams-container">
            `;

            // Автоматически генерируем карточки ладов для каждого уникального аккорда из песни
            if (trackMeta.used_chords && trackMeta.used_chords.length > 0) {
                trackMeta.used_chords.forEach(chord => {
                    const scheme = chordLibrarySVG[chord] || "Схема подбирается";
                    htmlCard += `
                        <div class="diagram-card">
                            <h5>${chord}</h5>
                            <div style="font-family: monospace; font-size: 1.1rem; color: #ffb64d; background: #121214; padding: 8px; border-radius: 4px;">
                                ${scheme}
                            </div>
                        </div>
                    `;
                });
            } else {
                // Если парсер вернул чистый текст, выводим базовые схемы Far From Any Road
                ["Am", "Em", "Dm", "C", "G", "F"].forEach(chord => {
                    htmlCard += `
                        <div class="diagram-card">
                            <h5>${chord}</h5>
                            <div style="font-family: monospace; font-size: 1.1rem; color: #ffb64d; background: #121214; padding: 8px; border-radius: 4px;">
                                ${chordLibrarySVG[chord] || "0 2 2 0 0 0"}
                            </div>
                        </div>
                    `;
                });
            }

            htmlCard += `</div></div>`;
            document.getElementById('songsResult').innerHTML = htmlCard;

            const audioComponent = document.getElementById('mainAudioComponent');
            audioComponent.src = audioUrl;

            document.getElementById('audioArtist').value = '';
            document.getElementById('audioTitle').value = '';
            fileInput.value = '';
        }
    })
    .catch(err => {
        alert('Не удалось выполнить обработку.');
        statusBox.innerText = '';
    });
}



// ФУНКЦИЯ ПОДДСВЕТКИ АККОРДОВ В РЕАЛЬНОМ ВРЕМЕНИ
function syncChordsWithAudio() {
    const audio = document.getElementById('mainAudioComponent');
    if (!audio || chordsDataGlobal.length === 0) return;

    const currentSeconds = Math.floor(audio.currentTime);

    // Находим, какой 5-секундный интервал звучит прямо сейчас
    let activeSeconds = 0;
    chordsDataGlobal.forEach(item => {
        if (currentSeconds >= item.seconds) {
            activeSeconds = item.seconds;
        }
    });

    // Сбрасываем подсветку со всех карточек и подсвечиваем текущую
    document.querySelectorAll('.chord-timeline-item').forEach(el => {
        el.style.background = '#121214';
        el.style.borderColor = '#29292e';
        el.style.transform = 'scale(1)';
    });

    const activeCard = document.getElementById(`chord-block-${activeSeconds}`);
    if (activeCard) {
        activeCard.style.background = 'rgba(0, 173, 181, 0.15)';
        activeCard.style.borderColor = '#00adb5';
        activeCard.style.transform = 'scale(1.03)';
    }
}

