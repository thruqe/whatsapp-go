package cliutils

import (
	"math/rand"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"

	utils "whatsrook/src"
)

var (
	BotJID         = types.NewJID("whatsrook_bot", "s.whatsapp.net")
	WcgRng         = rand.New(rand.NewSource(time.Now().UnixNano()))
	GameRng        = rand.New(rand.NewSource(time.Now().UnixNano()))
	GameHTTPClient = utils.NewHTTPClient(4 * time.Second)

	TTTMu    sync.Mutex
	TTTGames = make(map[string]*TTTGame)

	WCGDictionary = map[int][]string{
		3: {
			"cat", "dog", "sun", "pen", "box", "hat", "car", "run", "sky", "cup",
			"map", "fan", "bus", "key", "ice", "bed", "pin", "fox", "ant", "fly",
			"bat", "cow", "owl", "pig", "bar", "boy", "day", "jam", "net", "toy",
			"act", "air", "arm", "art", "ash", "bad", "bag", "ban", "bee", "bit",
			"bow", "bud", "bug", "cab", "cap", "dew", "dig", "dry", "ear", "egg",
			"end", "era", "eye", "far", "fit", "fog", "fun", "gem", "got", "gun",
		},
		4: {
			"book", "fish", "bird", "tree", "moon", "star", "fire", "wind", "rain", "snow",
			"door", "lamp", "desk", "ship", "frog", "lion", "duck", "bear", "gold", "ring",
			"blue", "pink", "rose", "leaf", "rock", "sand", "wave", "king", "hero", "game",
			"arch", "atom", "bark", "bell", "boat", "bolt", "camp", "cave", "city", "coin",
			"dark", "dawn", "deer", "echo", "edge", "farm", "fern", "flag", "glow", "hill",
			"hope", "iron", "lake", "lava", "mist", "nest", "path", "peak", "silk", "wolf",
		},
		5: {
			"apple", "bread", "chair", "clock", "earth", "house", "lemon", "music", "night", "ocean",
			"paper", "plant", "queen", "river", "robot", "snake", "table", "tiger", "train", "water",
			"beach", "cloud", "dance", "dream", "fruit", "green", "heart", "horse", "light", "magic",
			"amber", "angel", "arena", "arrow", "blade", "bloom", "brave", "bridge", "cabin", "camel",
			"charm", "chase", "chess", "coral", "crown", "eagle", "flame", "flute", "frost", "ghost",
			"grape", "honor", "island", "knife", "marsh", "melon", "orbit", "pearl", "storm", "valor",
		},
		6: {
			"animal", "banana", "bridge", "castle", "dragon", "engine", "flower", "forest", "garden", "island",
			"jungle", "monkey", "planet", "rabbit", "rocket", "silver", "spider", "stream", "sunset", "yellow",
			"buffer", "camera", "coffee", "doctor", "guitar", "laptop", "mirror", "number", "orange", "person",
			"anchor", "beacon", "breeze", "canyon", "cinder", "cobalt", "crater", "crystal", "falcon", "fossil",
			"galaxy", "glider", "harbor", "legend", "meadow", "meteor", "palace", "pebble", "pirate", "riddle",
			"shadow", "sphinx", "spirit", "statue", "summit", "tunnel", "velvet", "vortex", "willow", "winter",
		},
		7: {
			"airplane", "balloon", "blanket", "captain", "diamond", "dolphin", "feather", "giraffe", "hamster", "journey",
			"kitchen", "lantern", "monster", "octopus", "penguin", "pyramid", "rainbow", "silence", "thunder", "volcano",
			"battery", "biscuit", "chariot", "crystal", "freedom", "holiday", "kingdom", "painter", "scanner", "village",
			"alchemy", "avalanche", "blossom", "compass", "courage", "eclipse", "emerald", "glacier", "harmony", "horizon",
			"iceberg", "leopard", "monarch", "mystery", "orchard", "package", "panther", "pinnacle", "specter", "starlight",
			"sunrise", "surfer", "symphony", "texture", "trident", "twilight", "unicorn", "vampire", "whisper", "zephyr",
		},
		8: {
			"dinosaur", "elephant", "flamingo", "football", "hospital", "kangaroo", "mountain", "notebook", "painting", "umbrella",
			"universe", "building", "calendar", "computer", "firework", "fountain", "treasure", "sandwich", "squirrel", "triangle",
			"barbecue", "chemical", "champion", "downtown", "engineer", "marathon", "midnight", "passport", "starfish", "wildlife",
			"aquarium", "blizzard", "carousel", "cathedral", "corridor", "daffodil", "guardian", "hedgehog", "illusion", "infinity",
			"labyrinth", "moonlight", "navigator", "obsidian", "porcupine", "sculpture", "sentinel", "skeleton", "splendor", "starlight",
			"supernova", "tapestry", "telescope", "traverse", "umbrella", "velocity", "wanderer", "waterfall", "windstorm", "yearbook",
		},
		9: {
			"astronaut", "butterfly", "chocolate", "dandelion", "harmonica", "jellyfish", "lighthouse", "orchestra", "pineapple", "spaceship",
			"submarine", "sunflower", "telescope", "adventure", "almanac", "avalanche", "champagne", "firefighter", "landscape", "microscope",
			"nightmare", "sanctuary", "saxophone", "tarantula", "vegetable", "waterfall", "chameleon", "crocodile", "firehouse", "porcupine",
			"alchemist", "aqueduct", "artillery", "avalanches", "blaster", "boulevard", "catapult", "chrysanthemum", "constellation", "cormorant",
			"dragonfly", "gargoyle", "gladiator", "hologram", "hurricane", "kaleidoscope", "labyrinth", "meandering", "metropolis", "millennium",
			"moonstone", "narrative", "parachute", "quicksand", "reflection", "silhouette", "speakeasy", "swordfish", "tarantula", "whirlwind",
		},
		10: {
			"blacksmith", "centipede", "dictionary", "earthquake", "helicopter", "locomotive", "marshmallow", "motorcycle", "rainforest", "rollercoaster",
			"rollerblade", "skateboard", "strawberry", "trampoline", "underwater", "volleyball", "watermelons", "woodpecker", "cheesecake", "dermatology",
			"locomotion", "metropolis", "superhero", "tournament", "wandering", "wilderness", "friendship", "leadership", "playstation", "basketball",
			"abomination", "archaeology", "astronomer", "atmosphere", "benefactor", "caterpillar", "championship", "chandelier", "composition", "cuttlefish",
			"doppelganger", "encyclopedia", "extravaganza", "hovercraft", "hyperspace", "illumination", "masterpiece", "microscope", "mythology", "nightingale",
			"peppermint", "percussion", "phenomenon", "salamander", "sandcastle", "spacecraft", "strikethrough", "superpower", "thermometer", "wheelbarrow",
		},
		11: {
			"caterpillar", "destination", "electricity", "grasshopper", "illustration", "marshmallows", "masterpiece", "microphones", "neighborhood", "performance",
			"refrigerator", "skateboards", "snowboarding", "submarines", "supermarket", "telephones", "thunderstorm", "transformers", "waterfalls", "windmills",
			"acceleration", "aeronautics", "authenticity", "breathtaking", "camaraderie", "championship", "circumnavigate", "composition", "cryptography", "deliberation",
			"disappearance", "environment", "extraordinary", "fluorescence", "gravitation", "imagination", "independent", "kaleidoscopes", "mathematics", "microbiology",
			"multicolored", "nebulization", "observation", "perspective", "pharaohs", "philosopher", "proclamation", "roaring", "spectacular", "subconscious",
		},
		12: {
			"cheeseburgers", "constellation", "disappearances", "encyclopedia", "extraterrestrial", "huckleberry", "illustrations", "jurisdiction", "kindergarten", "locomotives",
			"microscopes", "neighborhoods", "organizations", "photographer", "refrigerators", "skateboarding", "supermarkets", "thunderstorms", "underground", "volleyballs",
			"annihilation", "apprehension", "architecture", "biodiversity", "championships", "chronological", "comprehension", "configuration", "conglomerate", "cryptographic",
			"determination", "discontented", "entertainment", "hallucination", "hydroelectric", "illumination", "impenetrable", "invertebrate", "jurisdiction", "kaleidoscopic",
			"malfunction", "manipulation", "microorganism", "misconception", "multiplication", "orchestrating", "photosynthesis", "preservation", "reconstruction", "unbelievable",
		},
		13: {
			"accomplishment", "archaeologist", "autobiography", "characteristics", "congratulations", "disappointment", "embarrassment", "encyclopedias", "extravaganza", "identification",
			"international", "investigation", "misunderstanding", "multiplication", "pharmaceutical", "qualification", "recommendation", "transformation", "transportation", "unpredictable",
			"biodegradable", "classification", "comprehensions", "conceptualize", "differentiation", "disagreeable", "discrimination", "electrostatic", "exemplification", "experimentation",
			"extravaganzas", "hallucinations", "hospitalization", "identification", "infrastructure", "interconnection", "interpretation", "microorganisms", "misdirection", "notwithstanding",
			"oversimplify", "photosynthesize", "predictability", "reclassification", "reconstruction", "reorganization", "standardization", "telecommunication", "underestimate", "unpredictable",
		},
		14: {
			"acknowledgment", "administrators", "characteristics", "classifications", "congratulation", "disadvantageous", "discrimination", "extravaganzas", "identification", "implementation",
			"individualism", "infrastructure", "interpretation", "investigations", "multiplications", "recommendations", "reconstruction", "representation", "responsibility", "transformations",
			"accountability", "biodegradability", "capitalization", "commercialization", "compartmentalize", "counterattacked", "declassification", "discontinuation", "disillusionment", "disproportional",
			"electromagnet", "experimentations", "hyperbolic", "immunoassays", "incomprehensible", "indestructible", "industrialization", "instrumentation", "intercontinental", "internationalize",
			"marginalization", "microbiology", "miscalculation", "misinterpretation", "overreacting", "personification", "proportionality", "rationalization", "rehabilitation", "telephones",
		},
		15: {
			"acknowledgments", "characterization", "congratulations", "counteroffensive", "disadvantages", "discontinuation", "experimentation", "identifications", "implementations", "incomprehensible",
			"indestructible", "infrastructures", "interpretations", "misunderstandings", "personification", "recommendation", "representations", "responsibilities", "standardization", "telecommunication",
			"anthropomorphic", "biodegradabilities", "commercialization", "compartmentalized", "conceptualization", "constitutionalism", "counterproductive", "decentralization", "disclassification", "disproportionate",
			"electrochemistry", "electromagnetism", "excommunication", "extemporaneous", "hyperventilation", "incompatibility", "incomprehensible", "indistinguishable", "industrialization", "institutionalized",
			"interchangeable", "intercontinental", "internationalization", "miscalculations", "misinterpretations", "multidimensional", "overcompensation", "photolithography", "rationalizations", "telecommunications",
		},
		16: {
			"characterizations", "counteroffensives", "discontinuations", "disillusionment", "electrocardiogram", "experimentations", "incomprehensible", "indestructibility", "industrialization", "institutionalized",
			"interchangeable", "intercontinental", "internationalize", "miscommunications", "misinterpretations", "personifications", "standardizations", "telecommunications", "unconstitutional", "unpredictability",
			"compartmentalizing", "comprehensiveness", "conceptualizations", "constitutionalized", "counterproductiveness", "decentralizations", "disclassifications", "electrochemically", "electromagnetisms", "excommunications",
			"hyperventilating", "incompatibilities", "incomprehensibility", "indistinguishably", "institutionalizing", "internationalizing", "microphotograph", "multidimensionality", "overcompensating", "oversimplification",
		},
	}
)

type TTTGame struct {
	Board          [9]string
	PlayerX        types.JID
	PlayerO        types.JID
	PlayerXMention types.JID
	PlayerOMention types.JID
	Turn           types.JID
	PlayerXTag     string
	PlayerOTag     string
	IsBotGame      bool
}

func IsTTTGameActive(chatJID string) bool {
	TTTMu.Lock()
	defer TTTMu.Unlock()
	_, exists := TTTGames[chatJID]
	return exists
}
