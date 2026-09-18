// Библиотека аппликатур (6 струна слева -> 1 струна справа)
const chordLibrarySVG = {
    "Am": "x 0 2 2 1 0", "C":  "x 3 2 0 1 0", "Em": "0 2 2 0 0 0", "G":  "3 2 0 0 0 3",
    "Dm": "x x 0 2 3 1", "F":  "1 3 3 2 1 1", "D":  "x x 0 2 3 2", "E":  "0 2 2 1 0 0",
    "Fm": "1 3 3 1 1 1", "B":  "x 2 4 4 4 2", "Bm": "x 2 4 4 3 2", "C#m": "x 4 6 6 5 4",
    "G#m": "4 6 6 4 4 4", "D#m": "x 6 8 8 7 6", "F#": "2 4 4 3 2 2"
};

const chromaticScale = ["C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"];

const arpeggioPatterns = [
    "🎸 Простая шестерка: Бас -> 3 -> 2 -> 1 -> 2 -> 3",
    "🎸 Восьмерка классическая: Бас -> 3 -> 2 -> 3 -> 1 -> 3 -> 2 -> 3",
    "🎸 Вальсовый перебор: Бас -> (3+2+1) -> (3+2+1)",
    "🎸 Четверка (Блатной): Бас -> 3 -> (2+1) -> 3"
];

const strumPatterns = {
    "44": [
        "⬇️ ⬇️ ⬆️ ⬆️ ⬇️ ⬆️ (Шестерка)",
        "⬇️ ❌ ⬆️ ⬆️ ❌ ⬆️ (Шестерка с глушением)",
        "⬇️ ❌ ⬆️ ⬇️ ❌ ⬆️ (Четверка)"
    ],
    "34": [
        "⬇️ ⬆️ ⬆️ (Вальсовый простой)",
        "⬇️ ❌ ❌ (Вальс с глушением)"
    ]
};

let generatedChordsCache = []; 
let currentActiveChordIndex = 0;
let audioCtx = null;
let isPlaying = false;
let practiceInterval = null;
let beatCount = 0;
let currentBpm = 120;

// При запуске страницы подгружаем историю из базы данных
document.addEventListener("DOMContentLoaded", () => {
    loadSequenceHistory();
});

// 1. Генерация аккордов по законам гармонии и настроениям
function generateChords() {
    const key = document.getElementById('chordKey').value;
    const mood = document.getElementById('chordMood').value;
    const level = document.getElementById('chordLevel').value;
    const length = document.getElementById('chordLength').value;

    fetch(`/api/chords/random?key=${key}&mood=${mood}&level=${level}&length=${length}`)
        .then(res => res.json())
        .then(data => {
            generatedChordsCache = data.sequence;
            renderChordsResult(generatedChordsCache);
        })
        .catch(err => console.error("Ошибка генерации:", err));
}

function renderChordsResult(chordsArray) {
    const box = document.getElementById('chordsResult');
    box.innerHTML = '';
    
    chordsArray.forEach((chord, idx) => {
        const rawScheme = chordLibrarySVG[chord] || "0 2 2 0 0 0";
        const shiftedScheme = calculateCapoScheme(rawScheme);

        box.innerHTML += `
            <div class="chord-box" id="chord-item-${idx}">
                <div style="font-size: 1.8rem; font-weight:700;">${chord}</div>
                <div style="font-family: monospace; font-size: 0.85rem; color: #a8a8b3; margin-top: 5px; letter-spacing: 1px;">
                    ${shiftedScheme}
                </div>
            </div>
        `;
    });
}

// 2. Транспонирование всей цепочки (+1 / -1 полутон)
function transposeChords(semitones) {
    if (generatedChordsCache.length === 0) return;

    generatedChordsCache = generatedChordsCache.map(chordName => {
        const isMinor = chordName.endsWith("m") && !chordName.endsWith("m7");
        const cleanName = isMinor ? chordName.slice(0, -1) : chordName;

        let idx = chromaticScale.indexOf(cleanName);
        if (idx === -1) return chordName;

        idx = (idx + semitones + 12) % 12;
        return chromaticScale[idx] + (isMinor ? "m" : "");
    });

    renderChordsResult(generatedChordsCache);
}

// 3. Вычисление схемы с учетом каподастра
function calculateCapoScheme(rawScheme) {
    const capo = parseInt(document.getElementById('capoFret').value) || 0;
    if (capo === 0) return rawScheme;

    return rawScheme.split(' ').map(fret => {
        if (fret === 'x' || fret === '0' || fret === '') return fret;
        let newFret = parseInt(fret) - capo;
        return newFret < 0 ? 0 : newFret;
    }).join(' ');
}

function applyCapoShift() {
    if (generatedChordsCache.length > 0) renderChordsResult(generatedChordsCache);
}

// 4. Генератор ритмических рисунков и переборов
function generateStrum() {
    const type = document.getElementById('strumType').value;
    let pool = [];

    if (type === "arpeggio") {
        pool = arpeggioPatterns;
    } else if (type === "all") {
        pool = [...strumPatterns["44"], ...strumPatterns["34"]];
    } else {
        pool = strumPatterns[type];
    }

    const randomIndex = Math.floor(Math.random() * pool.length);
    document.getElementById('strumResult').innerText = pool[randomIndex];
}

// 5. Умный тренажер практики (Таймер / Разгон)
function togglePracticeFields() {
    const mode = document.getElementById('practiceMode').value;
    document.getElementById('bpmField').style.display = mode === 'metronome' ? 'block' : 'none';
    document.getElementById('secondsField').style.display = mode === 'timer' ? 'block' : 'none';
}

function togglePracticeRoutine() {
    const btn = document.getElementById('metronomeBtn');
    const mode = document.getElementById('practiceMode').value;

    if (isPlaying) {
        clearTimeout(practiceInterval);
        isPlaying = false;
        btn.innerText = 'Старт тренировки';
        btn.style.background = '#00adb5';
        document.getElementById('beatProgressBar').style.width = '0%';
        return;
    }

    isPlaying = true;
    btn.innerText = 'Стоп';
    btn.style.background = '#ff4d4d';
    beatCount = 0;
    currentActiveChordIndex = 0;
    currentBpm = parseInt(document.getElementById('bpmInput').value) || 120;

    if (!audioCtx) audioCtx = new (window.AudioContext || window.webkitAudioContext)();

    if (mode === 'metronome') {
        runMetronomeEngine();
    } else {
        runTimerEngine();
    }
}

function runMetronomeEngine() {
    const tickTime = () => {
        if (!isPlaying) return;
        
        const intervalMs = (60 / currentBpm) * 1000;
        const isStrongBeat = (beatCount % 4 === 0);
        
        playClickSound(isStrongBeat ? 1200 : 700); 
        flashVisualProgressBar(intervalMs);

        if (isStrongBeat && generatedChordsCache.length > 0) {
            highlightActiveTrainingChord();
            
			// Разгон: каждые 4 такта (16 ударов) увеличиваем темп на +5 BPM
            if (beatCount > 0 && beatCount % 16 === 0 && document.getElementById('autoBpmCheck').checked) {
                currentBpm = Math.min(currentBpm + 5, 250);
                document.getElementById('bpmInput').value = currentBpm;
            }
        }

        beatCount++;
        practiceInterval = setTimeout(tickTime, intervalMs);
    };
    tickTime();
}

function runTimerEngine() {
    const seconds = parseInt(document.getElementById('secondsInput').value) || 4;
    const intervalMs = seconds * 1000;

    const tickTimer = () => {
        if (!isPlaying) return;
        playClickSound(900);
        flashVisualProgressBar(intervalMs);
        highlightActiveTrainingChord();
        practiceInterval = setTimeout(tickTimer, intervalMs);
    };
    
    highlightActiveTrainingChord();
    flashVisualProgressBar(intervalMs);
    practiceInterval = setTimeout(tickTimer, intervalMs);
}

function flashVisualProgressBar(ms) {
    const bar = document.getElementById('beatProgressBar');
    bar.style.transition = 'none';
    bar.style.width = '0%';
    setTimeout(() => {
        bar.style.transition = `width ${ms / 1000}s linear`;
        bar.style.width = '100%';
    }, 10);
}

function playClickSound(freq) {
    if (!audioCtx) return;
    const osc = audioCtx.createOscillator();
    const gainNode = audioCtx.createGain();
    osc.connect(gainNode);
    gainNode.connect(audioCtx.destination);
    osc.type = 'sine';
    osc.frequency.setValueAtTime(freq, audioCtx.currentTime);
    gainNode.gain.setValueAtTime(0.5, audioCtx.currentTime);
    gainNode.gain.exponentialRampToValueAtTime(0.001, audioCtx.currentTime + 0.04);
    osc.start();
    osc.stop(audioCtx.currentTime + 0.04);
}

function highlightActiveTrainingChord() {
    if (generatedChordsCache.length === 0) return;
    
    document.querySelectorAll('.chord-box').forEach(el => {
        el.style.borderColor = '#00adb5';
        el.style.transform = 'none';
    });
    
    const activeBox = document.getElementById(`chord-item-${currentActiveChordIndex}`);
    if (activeBox) {
        activeBox.style.borderColor = '#ffb64d'; 
        activeBox.style.transform = 'scale(1.05)';
    }
    
    currentActiveChordIndex = (currentActiveChordIndex + 1) % generatedChordsCache.length;
}

// 6. Сохранение цепочки и работа с базой данных
function saveCurrentSequence() {
    if (generatedChordsCache.length === 0) {
        alert("Сначала сгенерируйте аккорды!");
        return;
    }
    fetch('/api/chords/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ chords: generatedChordsCache })
    })
    .then(res => res.json())
    .then(() => {
        loadSequenceHistory();
    })
    .catch(err => console.error("Ошибка сохранения:", err));
}

function loadSequenceHistory() {
    const box = document.getElementById('historyResult');
    fetch('/api/chords/history')
        .then(res => res.json())
        .then(data => {
            if (!data || data.length === 0) {
                box.innerHTML = "История тренировок пока пуста.";
                return;
            }
            box.innerHTML = '';
            data.forEach(item => {
                box.innerHTML += `
                    <div style="background: #202024; padding: 10px 15px; border-radius: 6px; margin-bottom: 8px; border-left: 3px solid #ffb64d; display:flex; justify-content: space-between; align-items:center;">
                        <span style="font-weight:600; color:#fff; letter-spacing:1px;">${item.chords.join(' — ')}</span>
                        <button onclick='generatedChordsCache=${JSON.stringify(item.chords)}; renderChordsResult(generatedChordsCache);' style="padding: 4px 10px; font-size:0.8rem; background:#29292e; color:#00adb5; border:1px solid #00adb5;">Загрузить</button>
                    </div>
                `;
});
})
.catch(() => {
    box.innerHTML = "Не удалось загрузить историю база данных отключена или настроена";
    });
}